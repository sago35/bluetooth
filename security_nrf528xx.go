//go:build (softdevice && s113v7) || (softdevice && s132v6) || (softdevice && s140v6) || (softdevice && s140v7)

package bluetooth

import (
	"crypto/ecdh"
	"errors"
	"runtime/volatile"
	"time"
)

/*
#include "ble.h"
#include "ble_gap.h"
#include "ble_gatts.h"
#include "nrf_soc.h"
*/
import "C"

var (
	errPairingNotEnabled = errors.New("bluetooth: pairing is not enabled")
	errInvalidPasskey    = errors.New("bluetooth: passkey must be 6 ASCII digits")
)

// Security event flags, set from the SoftDevice event handler (interrupt
// context) and consumed by the security worker goroutine.
const (
	flagSecParamsRequest = 1 << iota
	flagPasskeyDisplay
	flagNumericComparison
	flagAuthKeyRequest
	flagLESCDHKeyRequest
	flagAuthStatus
	flagSecInfoRequest
)

// idleAuthTimeout is how long a connected central has to encrypt the link
// (by pairing or by resuming a bond) before it is disconnected.
const idleAuthTimeout = 10 * time.Second

// Static buffers for the pairing procedure. The SoftDevice requires the
// keyset memory to stay valid until the BLE_GAP_EVT_AUTH_STATUS event, so
// package-level variables are used. This limits pairing to one procedure at a
// time, matching the single-connection design of this backend.
var (
	pairingConfig  PairingParams
	pairingEnabled volatile.Register8
	workerStarted  bool

	secParamsReply C.ble_gap_sec_params_t
	secKeyset      C.ble_gap_sec_keyset_t
	ownPubkey      C.ble_gap_lesc_p256_pk_t // our LESC public key, in SMP format
	peerPubkey     C.ble_gap_lesc_p256_pk_t // the SoftDevice stores the peer key here
	lescDHKey      C.ble_gap_lesc_dhkey_t
	lescPrivateKey *ecdh.PrivateKey
	staticPasskey  [6]C.uint8_t
	authKeyBuf     [6]C.uint8_t

	// Mailbox from the event handler to the security worker. The buffers are
	// written before the flag is set, and the flag is cleared inside a
	// critical region.
	secEventFlags    volatile.Register8
	secEventConn     = volatileHandle{handle: volatile.Register16{C.BLE_CONN_HANDLE_INVALID}}
	passkeyBuf       [6]C.uint8_t
	authStatusCode   volatile.Register8
	authStatusBonded volatile.Register8
	// authStatusLESC reports whether the pairing procedure used LE Secure
	// Connections, as opposed to legacy pairing. This is independent of
	// authStatusBonded: legacy pairing can bond too, and the CONN_SEC_UPDATE
	// security level alone doesn't distinguish LESC Just Works (level 2)
	// from legacy Just Works (also level 2). Only used for the debug log.
	authStatusLESC  volatile.Register8
	secInfoMasterID C.ble_gap_master_id_t
	secInfoEncReq   volatile.Register8

	// The LTK from the last bonding procedure, persisted to flash (see
	// bond_storage_nrf528xx.go). bondValid reports whether it is populated.
	// It is written by the security worker, by RemoveBond (application
	// goroutine), and by loadBondFromFlash (called from EnablePairing), and
	// read from interrupt context (secOnSecParamsRequest) and the security
	// worker, so it must be a volatile type like the rest of this shared
	// state.
	ownEncKey C.ble_gap_enc_key_t
	bondValid volatile.Register8

	// allowNewPairing gates pairing with a central this device does not
	// already have a bond with. It is closed (0) while the active slot
	// holds a bond, so a nearby device cannot silently take over an
	// already-paired keyboard, and opens automatically whenever the active
	// slot is empty (on load and on SelectBondSlot: selecting an empty
	// slot, like unpairing, is the explicit "pair a new device" action).
	// It closes again as soon as a new bond is formed. The application can
	// override it at any time with Adapter.AllowNewPairing.
	allowNewPairing volatile.Register8

	// System attributes (CCCD values) of the bonded central, saved at
	// disconnect. A bonded central expects its notification subscriptions to
	// persist across reconnections and does not necessarily subscribe again.
	// peerBonded reports whether the currently connected central is the one
	// we share the bond with.
	sysAttrBuf [64]C.uint8_t
	sysAttrLen volatile.Register16
	peerBonded volatile.Register8

	// secInfoKeyReplied is set when the security worker answered a SEC_INFO
	// request with our bonded LTK, and consumed by secOnConnSecUpdate: only
	// once the link actually encrypted is the central treated as the bond's
	// owner (peerBonded). Setting peerBonded already at the reply would let
	// a failed encryption attempt - e.g. a central bonded to another slot,
	// whose LESC master ID is indistinguishable from ours - count as bonded
	// and have its junk CCCD state saved over the active slot's.
	secInfoKeyReplied volatile.Register8

	// connPending is set while a central is connected but has not yet
	// encrypted the link (by pairing or by resuming a bond). The security
	// worker disconnects it if this takes longer than idleAuthTimeout, so a
	// central that never attempts to pair cannot occupy the single
	// connection slot forever and lock out the bonded central.
	connPending       volatile.Register8
	pendingConnHandle = volatileHandle{handle: volatile.Register16{C.BLE_CONN_HANDLE_INVALID}}
	// bondDirty is set (from interrupt context) whenever the RAM copy of the
	// bond (sys attrs) changes and needs to be written back to flash. It is
	// consumed by the bond storage worker: flash writes block for tens of
	// milliseconds, so they must not run in interrupt context nor hold up
	// the security worker's handling of pairing events.
	bondDirty volatile.Register8
	// Static because taking the address of a local variable for a C call
	// would heap-allocate, which is not allowed in interrupt context.
	sysAttrGetLen C.uint16_t
)

// BondSlotCount is the number of independent bond slots ("profiles") this
// backend can persist. See SelectBondSlot.
const BondSlotCount = 5

// bondSlot is the RAM cache of one persisted bond slot. Only the active
// slot is mirrored into the live single-bond state above (ownEncKey,
// bondValid, sysAttrBuf/sysAttrLen): LESC master IDs are all zero, so a
// SEC_INFO request cannot tell bonded peers apart, and without IRK support
// neither can a connection. Keeping exactly one bond live at a time (like
// ZMK profiles) sidesteps that: only the active slot's central can
// re-encrypt; another slot's central that connects fails encryption and
// disconnects, with its own copy of the bond intact.
type bondSlot struct {
	valid      bool
	encKey     C.ble_gap_enc_key_t
	sysAttrLen uint16
	sysAttr    [64]byte
}

var (
	bondSlots      [BondSlotCount]bondSlot
	activeBondSlot volatile.Register8

	// bondSwitching gates new connections while SelectBondSlot rearranges
	// the bond state: advertising restarts as soon as the old central is
	// disconnected, and a central connecting mid-switch could encrypt
	// against the old slot's key and then have its CCCD state saved into
	// the new slot.
	bondSwitching volatile.Register8

	// SelectBondSlot -> bond storage worker handshake, in the same style as
	// the erase handshake in bond_storage_nrf528xx.go: bondSwitchErr is only
	// valid once bondSwitchDone is set, and only one switch can be
	// outstanding at a time (SelectBondSlot blocks until completion).
	bondSwitchRequested volatile.Register8
	bondSwitchTarget    volatile.Register8
	bondSwitchDone      volatile.Register8
	bondSwitchErr       error
)

var (
	errInvalidBondSlot   = errors.New("bluetooth: invalid bond slot")
	errBondSwitchTimeout = errors.New("bluetooth: bond slot switch: disconnect did not complete")
)

// syncActiveSlotFromState copies the live single-bond state into the active
// slot's cache entry, so that a subsequent page write persists it.
func syncActiveSlotFromState() {
	s := &bondSlots[activeBondSlot.Get()]
	s.valid = bondValid.Get() != 0
	s.encKey = ownEncKey
	s.sysAttrLen = sysAttrLen.Get()
	for i := range s.sysAttr {
		s.sysAttr[i] = byte(sysAttrBuf[i])
	}
}

// loadStateFromSlot loads a slot's cache entry into the live single-bond
// state. The caller must make sure no connection is using that state.
func loadStateFromSlot(n int) {
	s := &bondSlots[n]
	ownEncKey = s.encKey
	for i := range sysAttrBuf {
		sysAttrBuf[i] = C.uint8_t(s.sysAttr[i])
	}
	sysAttrLen.Set(s.sysAttrLen)
	if s.valid {
		bondValid.Set(1)
	} else {
		bondValid.Set(0)
	}
}

// EnablePairing configures the adapter to accept pairing and bonding requests
// from a connected central. It must be called after Enable() and before a
// central connects. Without it, incoming pairing requests are rejected with
// "pairing not supported".
//
// Bonds are persisted to flash, so a bonded central can re-encrypt the link
// on reconnection without pairing again, even after a reset of this device.
// Up to BondSlotCount bonds are kept, one per slot, but only the active
// slot's bond is live at any time (see SelectBondSlot); pairing with a new
// central overwrites the active slot.
func (a *Adapter) EnablePairing(params PairingParams) error {
	// Always set BLE_GAP_OPT_PASSKEY, even when StaticPasskey is empty:
	// passing a NULL p_passkey tells the SoftDevice to go back to generating a
	// random passkey. Without this, a static passkey configured by an earlier
	// EnablePairing call would stay active.
	var passkeyPtr *C.uint8_t
	if params.StaticPasskey != "" {
		if !isValidPasskey(params.StaticPasskey) {
			return errInvalidPasskey
		}
		for i := 0; i < 6; i++ {
			staticPasskey[i] = C.uint8_t(params.StaticPasskey[i])
		}
		passkeyPtr = &staticPasskey[0]
	}
	var opt C.ble_opt_t
	opt.unionfield_gap_opt().unionfield_passkey().p_passkey = passkeyPtr
	errCode := C.sd_ble_opt_set(C.BLE_GAP_OPT_PASSKEY, &opt)
	if errCode != 0 {
		return Error(errCode)
	}

	secParamsReply = C.ble_gap_sec_params_t{
		min_key_size: 7,
		max_key_size: 16,
	}
	// Advertise bonding support: common centrals (Windows, iOS) do not
	// complete their pairing flow without a bond.
	secParamsReply.set_bitfield_bond(1)
	secParamsReply.kdist_own.set_bitfield_enc(1)
	if params.MITM {
		secParamsReply.set_bitfield_mitm(1)
	}
	if params.LESC {
		secParamsReply.set_bitfield_lesc(1)
	}
	secParamsReply.set_bitfield_io_caps(params.IOCapabilities.sdIOCaps())
	secParamsReply.set_bitfield_oob(0)

	// The SoftDevice stores keys generated during pairing in application
	// memory referenced by this keyset. The own encryption key receives the
	// LTK (distributed by us in legacy pairing, generated locally in LESC)
	// that a bonded central uses to re-encrypt the link on reconnection.
	// keys_peer.p_enc_key must stay NULL for LESC and is not needed in the
	// peripheral role.
	secKeyset.keys_own.p_enc_key = &ownEncKey
	secKeyset.keys_own.p_pk = &ownPubkey
	secKeyset.keys_peer.p_pk = &peerPubkey

	if params.LESC {
		if err := lescGenerateKeypair(); err != nil {
			return err
		}
	}

	if !workerStarted {
		// Load a bond persisted by a previous run, if any, before the first
		// connection can arrive.
		loadBondFromFlash()
	}

	pairingConfig = params
	if !workerStarted {
		workerStarted = true
		go securityWorker()
		go bondStorageWorker()
	}
	pairingEnabled.Set(1)
	return nil
}

// AllowNewPairing opens or closes the pairing window for a central this
// device does not already have a bond with. The window is managed
// automatically - open while the active slot is empty (including right
// after RemoveBond or after selecting an empty slot), closed as soon as a
// bond is formed - so most applications never need to call this. Call it to
// override that default, e.g. to let a new central take over a bonded slot
// without unpairing first, typically gated behind a physical action such as
// a particular combination of buttons held at startup.
func (a *Adapter) AllowNewPairing(allow bool) {
	if allow {
		allowNewPairing.Set(1)
	} else {
		allowNewPairing.Set(0)
	}
}

// RemoveBond deletes the bond stored in the active slot, both the RAM copy
// and the flash record, and disconnects the currently connected central (if
// any). The previously bonded central can then no longer re-encrypt. Bonds
// in other slots are not affected. RemoveBond also opens the pairing window
// (see AllowNewPairing): it is the "unpair" action that frees the active
// slot for a new central.
//
// It must be called from goroutine context (it blocks until the flash erase
// has completed), never from an interrupt.
func (a *Adapter) RemoveBond() error {
	bondValid.Set(0)
	// peerBonded must be cleared too: sd_ble_gap_disconnect below is
	// asynchronous, so the actual disconnect event can still arrive after
	// this function returns. If peerBonded were left set, secOnDisconnect
	// would save the (already-cleared, but re-readable from the SoftDevice)
	// CCCD state again and the bond storage worker would write a bond
	// record back to flash right after this function erased it.
	peerBonded.Set(0)
	bondDirty.Set(0)
	sysAttrLen.Set(0)
	ownEncKey = C.ble_gap_enc_key_t{}
	for i := range sysAttrBuf {
		sysAttrBuf[i] = 0
	}

	connHandle := currentConnection.Get()
	if connHandle != C.BLE_CONN_HANDLE_INVALID {
		if err := makeError(C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_REMOTE_USER_TERMINATED_CONNECTION)); err != nil {
			return err
		}
	}

	// Unpairing means "make this slot available for a new central", so open
	// the pairing window; it closes again as soon as a new bond is formed.
	allowNewPairing.Set(1)

	if !workerStarted {
		// EnablePairing was never called, so the bond storage worker isn't
		// running and no other goroutine can touch the flash: erase directly.
		return eraseBondFromFlash()
	}
	// Hand the erase to the bond storage worker, which owns all bond flash
	// operations, so it cannot race with a save that may be in progress.
	bondEraseDone.Set(0)
	bondEraseRequested.Set(1)
	for bondEraseDone.Get() == 0 {
		time.Sleep(time.Millisecond)
	}
	return bondEraseErr
}

// SelectBondSlot switches the active bond slot ("profile", 0 to
// BondSlotCount-1). Each slot holds an independent bond, but only the active
// one is live: its central can reconnect and re-encrypt, while a central
// bonded to another slot fails encryption and disconnects (keeping its own
// bond record intact, so it works again when its slot becomes active).
// The currently connected central, if any, is disconnected first.
//
// Selecting an empty slot opens the pairing window, so a new central can
// pair into it right away; selecting a bonded slot closes the window (see
// allowNewPairing).
//
// The active slot number itself is not persisted; the application selects
// the slot it wants at startup. Called before EnablePairing, this only picks
// the slot that EnablePairing will load from flash. Called after, it must
// run in goroutine context: it blocks until the switch completed, including
// waiting out the disconnect (up to a few seconds).
func (a *Adapter) SelectBondSlot(n int) error {
	if n < 0 || n >= BondSlotCount {
		return errInvalidBondSlot
	}
	if int(activeBondSlot.Get()) == n {
		return nil
	}
	if !workerStarted {
		// EnablePairing has not run yet: the slots are not loaded and there
		// is nothing live to swap out, so just pick the slot for
		// loadBondFromFlash to activate later. Writing flash here would
		// wipe the stored bonds with the empty RAM cache.
		activeBondSlot.Set(uint8(n))
		return nil
	}

	// Keep new connections away while the bond state is inconsistent (see
	// bondSwitching): advertising resumes right after the disconnect below.
	bondSwitching.Set(1)
	defer bondSwitching.Set(0)

	// Disconnect the current central and wait for the disconnect to
	// complete: the DISCONNECTED handler saves the connection's CCCD state
	// into the still-active old slot. Swapping before it ran would leak the
	// old central's CCCD state into the new slot.
	if connHandle := currentConnection.Get(); connHandle != C.BLE_CONN_HANDLE_INVALID {
		// The result is not checked on purpose: the connection may be going
		// away on its own right now, and the wait below covers every case.
		C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_REMOTE_USER_TERMINATED_CONNECTION)
		ok := false
		for i := 0; i < 3000; i++ { // supervision timeouts run in seconds
			if currentConnection.Get() == C.BLE_CONN_HANDLE_INVALID {
				ok = true
				break
			}
			time.Sleep(time.Millisecond)
		}
		if !ok {
			return errBondSwitchTimeout
		}
	}

	// Hand the actual switch to the bond storage worker, which owns all
	// bond flash operations and the slot cache.
	bondSwitchDone.Set(0)
	bondSwitchTarget.Set(uint8(n))
	bondSwitchRequested.Set(1)
	for bondSwitchDone.Get() == 0 {
		time.Sleep(time.Millisecond)
	}
	return bondSwitchErr
}

// BondSlot returns the active bond slot selected with SelectBondSlot.
func (a *Adapter) BondSlot() int {
	return int(activeBondSlot.Get())
}

// RequestPairing sends a security request to the connected central, asking it
// to initiate pairing. The central may ignore the request. EnablePairing must
// have been called first.
func (d Device) RequestPairing() error {
	if pairingEnabled.Get() == 0 {
		return errPairingNotEnabled
	}
	errCode := C.sd_ble_gap_authenticate(d.connectionHandle, &secParamsReply)
	return makeError(errCode)
}

// EnterPasskey supplies the 6-digit passkey shown by the central, in response
// to PairingParams.PasskeyEntryHandler. Pass an empty string to reject the
// pairing.
func (d Device) EnterPasskey(passkey string) error {
	if passkey == "" {
		errCode := C.sd_ble_gap_auth_key_reply(d.connectionHandle, C.BLE_GAP_AUTH_KEY_TYPE_NONE, nil)
		return makeError(errCode)
	}
	if !isValidPasskey(passkey) {
		return errInvalidPasskey
	}
	for i := 0; i < 6; i++ {
		authKeyBuf[i] = C.uint8_t(passkey[i])
	}
	errCode := C.sd_ble_gap_auth_key_reply(d.connectionHandle, C.BLE_GAP_AUTH_KEY_TYPE_PASSKEY, &authKeyBuf[0])
	return makeError(errCode)
}

// ConfirmPasskey answers a numeric comparison request from
// PairingParams.PasskeyComparisonHandler: match reports whether the user
// confirmed that both devices show the same passkey.
func (d Device) ConfirmPasskey(match bool) error {
	keyType := C.uint8_t(C.BLE_GAP_AUTH_KEY_TYPE_NONE)
	if match {
		keyType = C.BLE_GAP_AUTH_KEY_TYPE_PASSKEY
	}
	errCode := C.sd_ble_gap_auth_key_reply(d.connectionHandle, keyType, nil)
	return makeError(errCode)
}

func isValidPasskey(passkey string) bool {
	if len(passkey) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if passkey[i] < '0' || passkey[i] > '9' {
			return false
		}
	}
	return true
}

func (io IOCapabilities) sdIOCaps() C.uint8_t {
	switch io {
	case IOCapsDisplayOnly:
		return C.BLE_GAP_IO_CAPS_DISPLAY_ONLY
	case IOCapsDisplayYesNo:
		return C.BLE_GAP_IO_CAPS_DISPLAY_YESNO
	case IOCapsKeyboardOnly:
		return C.BLE_GAP_IO_CAPS_KEYBOARD_ONLY
	case IOCapsKeyboardDisplay:
		return C.BLE_GAP_IO_CAPS_KEYBOARD_DISPLAY
	default:
		return C.BLE_GAP_IO_CAPS_NONE
	}
}

// The following functions are called from the SoftDevice event handler, in
// interrupt context. They only record the event for the security worker,
// except for replies that need no application input (which are single SVC
// calls, like the other replies already done in the event handler).

func secSetFlag(flag uint8) {
	secEventFlags.Set(secEventFlags.Get() | flag)
}

func secClearFlag(flag uint8) {
	mask := DisableInterrupts()
	secEventFlags.Set(secEventFlags.Get() &^ flag)
	RestoreInterrupts(mask)
}

func secOnConnect() {
	peerBonded.Set(0)
	secInfoKeyReplied.Set(0)
}

// secOnConnectPeripheral additionally starts the idle-auth disconnect timer
// (see connPending). Bonding is a peripheral-side concept in this library, so
// this must only be called for a connection where this device is the
// peripheral, not for an outgoing central-role connection.
func secOnConnectPeripheral(connHandle C.uint16_t) {
	if pairingEnabled.Get() == 0 {
		return
	}
	if bondSwitching.Get() != 0 ||
		(bondValid.Get() == 0 && allowNewPairing.Get() == 0) {
		// An empty active slot with the pairing window closed can serve
		// nobody, but a central bonded to another slot may auto-reconnect
		// to it: answering its SEC_INFO request with "no keys" would make
		// it consider its own bond broken. Disconnect right away instead;
		// the central keeps its bond and simply retries later. The same
		// applies while SelectBondSlot is rearranging the bond state.
		if debug {
			println("disconnecting: no bond to serve on the active slot")
		}
		C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_REMOTE_USER_TERMINATED_CONNECTION)
		return
	}
	pendingConnHandle.Set(connHandle)
	connPending.Set(1)
}

// secSaveSysAttrs saves the CCCD state of the bonded central so it can be
// restored when it reconnects. It is called on every GATT server write (which
// includes subscription changes) and at disconnect.
func secSaveSysAttrs(connHandle C.uint16_t) {
	if peerBonded.Get() == 0 || pairingEnabled.Get() == 0 {
		return
	}
	sysAttrGetLen = C.uint16_t(len(sysAttrBuf))
	errCode := C.sd_ble_gatts_sys_attr_get(connHandle, &sysAttrBuf[0], &sysAttrGetLen, 0)
	if errCode == 0 {
		sysAttrLen.Set(uint16(sysAttrGetLen))
		// The central's subscriptions are part of the persisted bond, so a
		// bonded central does not have to subscribe again after a reset of
		// this device. Persisting is deferred to the security worker.
		bondDirty.Set(1)
	}
	if debug {
		println("sys attr save:", errCode, "len:", uint16(sysAttrGetLen))
	}
}

func secOnDisconnect(connHandle C.uint16_t) {
	secSaveSysAttrs(connHandle)
	connPending.Set(0)
}

// secRestoreSysAttrs restores the saved CCCD state of the bonded central, and
// reports whether it did.
func secRestoreSysAttrs(connHandle C.uint16_t) bool {
	if peerBonded.Get() == 0 || sysAttrLen.Get() == 0 {
		return false
	}
	errCode := C.sd_ble_gatts_sys_attr_set(connHandle, &sysAttrBuf[0], C.uint16_t(sysAttrLen.Get()), 0)
	if debug {
		println("sys attr restore:", errCode)
	}
	return errCode == 0
}

// secOnConnSecUpdate is called when the link encryption changed. When the
// bonded central re-encrypted the link, its saved CCCD state is restored
// right away: the central expects its subscriptions to persist, so waiting
// for a BLE_GATTS_EVT_SYS_ATTR_MISSING event is not enough (sending a
// notification does not generate that event, it just fails).
func secOnConnSecUpdate(connHandle C.uint16_t) {
	// The link is now encrypted, one way or another: cancel the idle-auth
	// disconnect timer.
	connPending.Set(0)
	if secInfoKeyReplied.Get() != 0 {
		// The central re-encrypted successfully with our bonded LTK: it is
		// the bond's owner (see secInfoKeyReplied for why this must not
		// happen earlier). Pairing-based encryption sets peerBonded via
		// AUTH_STATUS instead.
		secInfoKeyReplied.Set(0)
		peerBonded.Set(1)
	}
	secRestoreSysAttrs(connHandle)
}

func secOnSysAttrMissing(connHandle C.uint16_t) {
	if !secRestoreSysAttrs(connHandle) {
		C.sd_ble_gatts_sys_attr_set(connHandle, nil, 0, 0)
	}
}

func secOnSecParamsRequest(connHandle C.uint16_t) {
	if pairingEnabled.Get() == 0 {
		// Pairing is not configured: politely reject instead of letting the
		// central wait for the SMP timeout.
		if debug {
			println("pairing rejected: pairing not enabled")
		}
		C.sd_ble_gap_sec_params_reply(connHandle, C.BLE_GAP_SEC_STATUS_PAIRING_NOT_SUPP, nil, nil)
		return
	}
	if bondValid.Get() != 0 && allowNewPairing.Get() == 0 {
		// Already bonded with a central and not in pairing mode: reject a
		// new pairing attempt instead of letting a nearby device take over.
		if debug {
			println("pairing rejected: already bonded and pairing mode is closed")
		}
		C.sd_ble_gap_sec_params_reply(connHandle, C.BLE_GAP_SEC_STATUS_PAIRING_NOT_SUPP, nil, nil)
		return
	}
	secEventConn.Set(connHandle)
	secSetFlag(flagSecParamsRequest)
}

func secOnSecInfoRequest(connHandle C.uint16_t, evt *C.ble_gap_evt_sec_info_request_t) {
	if pairingEnabled.Get() == 0 {
		// No pairing support configured, so no keys can be stored: reply that
		// the keys are lost so the central can re-pair.
		C.sd_ble_gap_sec_info_reply(connHandle, nil, nil, nil)
		return
	}
	secInfoMasterID = evt.master_id
	secInfoEncReq.Set(uint8(evt.bitfield_enc_info()))
	secEventConn.Set(connHandle)
	secSetFlag(flagSecInfoRequest)
}

func secOnPasskeyDisplay(connHandle C.uint16_t, evt *C.ble_gap_evt_passkey_display_t) {
	passkeyBuf = evt.passkey
	secEventConn.Set(connHandle)
	// match_request is a single bitfield, which TinyGo's CGo exposes as a
	// plain byte field (bit 0).
	if evt.match_request&1 != 0 {
		secSetFlag(flagNumericComparison)
	} else {
		secSetFlag(flagPasskeyDisplay)
	}
}

func secOnAuthKeyRequest(connHandle C.uint16_t) {
	secEventConn.Set(connHandle)
	secSetFlag(flagAuthKeyRequest)
}

func secOnLESCDHKeyRequest(connHandle C.uint16_t) {
	// The peer public key has been stored by the SoftDevice in peerPubkey
	// (via the keyset), so only the request itself must be recorded.
	secEventConn.Set(connHandle)
	secSetFlag(flagLESCDHKeyRequest)
}

func secOnAuthStatus(connHandle C.uint16_t, evt *C.ble_gap_evt_auth_status_t) {
	if pairingEnabled.Get() == 0 {
		return
	}
	authStatusCode.Set(uint8(evt.auth_status))
	authStatusBonded.Set(uint8(evt.bitfield_bonded()))
	authStatusLESC.Set(uint8(evt.bitfield_lesc()))
	secEventConn.Set(connHandle)
	secSetFlag(flagAuthStatus)
}

// securityWorker runs all pairing replies and application callbacks outside
// interrupt context: replies may need to be retried (NRF_ERROR_BUSY), the
// LESC DHKey computation is too slow for an interrupt handler, and callbacks
// may block on user input.
func securityWorker() {
	const pollInterval = 16 * time.Millisecond
	var pendingElapsed time.Duration
	for {
		flags := secEventFlags.Get()
		if flags == 0 {
			if connPending.Get() == 0 {
				pendingElapsed = 0
			} else {
				pendingElapsed += pollInterval
				if pendingElapsed >= idleAuthTimeout {
					pendingElapsed = 0
					connPending.Set(0)
					if debug {
						println("disconnecting: link was never encrypted")
					}
					C.sd_ble_gap_disconnect(pendingConnHandle.Get(), C.BLE_HCI_LOCAL_HOST_TERMINATED_CONNECTION)
				}
			}
			time.Sleep(pollInterval)
			continue
		}
		connHandle := secEventConn.Get()
		device := Device{connectionHandle: connHandle}
		switch {
		case flags&flagSecParamsRequest != 0:
			secClearFlag(flagSecParamsRequest)
			for {
				errCode := C.sd_ble_gap_sec_params_reply(connHandle, C.BLE_GAP_SEC_STATUS_SUCCESS, &secParamsReply, &secKeyset)
				if errCode != 17 { // C.NRF_ERROR_BUSY, which TinyGo's CGo cannot parse
					if debug && errCode != 0 {
						println("sec params reply failed:", errCode)
					}
					break
				}
				time.Sleep(time.Millisecond)
			}
		case flags&flagPasskeyDisplay != 0:
			secClearFlag(flagPasskeyDisplay)
			if handler := pairingConfig.PasskeyDisplayHandler; handler != nil {
				handler(device, passkeyString())
			}
		case flags&flagNumericComparison != 0:
			secClearFlag(flagNumericComparison)
			if handler := pairingConfig.PasskeyComparisonHandler; handler != nil {
				handler(device, passkeyString())
			} else {
				// No way to ask the user: fail closed.
				device.ConfirmPasskey(false)
			}
		case flags&flagAuthKeyRequest != 0:
			secClearFlag(flagAuthKeyRequest)
			if handler := pairingConfig.PasskeyEntryHandler; handler != nil {
				handler(device)
			} else {
				device.EnterPasskey("")
			}
		case flags&flagLESCDHKeyRequest != 0:
			secClearFlag(flagLESCDHKeyRequest)
			lescComputeAndReply(connHandle)
		case flags&flagAuthStatus != 0:
			secClearFlag(flagAuthStatus)
			var err error
			if code := authStatusCode.Get(); code != C.BLE_GAP_SEC_STATUS_SUCCESS {
				err = PairingError(code)
			}
			if debug {
				println("evt: gap auth status: bonded", authStatusBonded.Get() != 0, "lesc", authStatusLESC.Get() != 0)
			}
			if err == nil && authStatusBonded.Get() != 0 {
				bondValid.Set(1)
				peerBonded.Set(1)
				// A new bond invalidates CCCD state saved for an old one.
				sysAttrLen.Set(0)
				bondDirty.Set(1)
				// Close the pairing window again: it only ever admits one
				// new bond. A failed attempt (the other branch) leaves an
				// existing bond, and the pairing window it required, alone.
				allowNewPairing.Set(0)
			} else {
				bondValid.Set(0)
			}
			if handler := pairingConfig.PairingCompleteHandler; handler != nil {
				handler(device, err)
			}
		case flags&flagSecInfoRequest != 0:
			secClearFlag(flagSecInfoRequest)
			// A central that bonded with us before asks to encrypt the link
			// with the LTK identified by the master ID (all zero for LESC).
			if bondValid.Get() != 0 && secInfoEncReq.Get() != 0 &&
				secInfoMasterID.ediv == ownEncKey.master_id.ediv &&
				secInfoMasterID.rand == ownEncKey.master_id.rand {
				// peerBonded is only set once the encryption actually
				// succeeds (see secInfoKeyReplied): under LESC the all-zero
				// master ID also matches a central bonded to another slot,
				// whose encryption attempt then fails with a MIC error - it
				// disconnects and keeps its own bond intact.
				secInfoKeyReplied.Set(1)
				errCode := C.sd_ble_gap_sec_info_reply(connHandle, &ownEncKey.enc_info, nil, nil)
				if debug && errCode != 0 {
					println("sec info reply failed:", errCode)
				}
			} else {
				// No key for this central on the active slot. It has a bond
				// (a central without one pairs via a SEC_PARAMS request and
				// never sends SEC_INFO), most likely with another slot - or
				// with a slot that was just unpaired. Replying "keys lost"
				// would make it discard its own copy of the bond, which must
				// survive until its slot is active again: disconnect
				// instead. A central whose bond really is gone (unpaired on
				// this device) has to be removed on the central side too,
				// after which it pairs anew via SEC_PARAMS.
				if debug {
					println("disconnecting: sec info request for a bond we don't hold")
				}
				C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_REMOTE_USER_TERMINATED_CONNECTION)
			}
		}
	}
}

func passkeyString() string {
	var passkey [6]byte
	for i := 0; i < 6; i++ {
		passkey[i] = byte(passkeyBuf[i])
	}
	return string(passkey[:])
}

// sdRand fills the buffer with random bytes from the SoftDevice random pool.
// The pool is smaller than a P-256 key seed and refills slowly, so the buffer
// is filled in as many steps as needed. The RNG peripheral itself is owned by
// the SoftDevice, so crypto/rand must not be used.
func sdRand(buf []byte) {
	for len(buf) > 0 {
		var available C.uint8_t
		C.sd_rand_application_bytes_available_get(&available)
		n := int(available)
		if n == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		if n > len(buf) {
			n = len(buf)
		}
		if C.sd_rand_application_vector_get((*C.uint8_t)(&buf[0]), C.uint8_t(n)) == 0 {
			buf = buf[n:]
		}
	}
}

// lescGenerateKeypair generates the P-256 keypair used for LE Secure
// Connections and stores the public key in ownPubkey, in the SMP format the
// SoftDevice expects: X and Y coordinates, both little-endian.
func lescGenerateKeypair() error {
	var seed [32]byte
	for {
		sdRand(seed[:])
		privateKey, err := ecdh.P256().NewPrivateKey(seed[:])
		if err != nil {
			// The seed was out of range for a P-256 scalar; try again.
			continue
		}
		lescPrivateKey = privateKey
		// SEC1 uncompressed format: 0x04 || X (big-endian) || Y (big-endian).
		publicKey := privateKey.PublicKey().Bytes()
		for i := 0; i < 32; i++ {
			ownPubkey.pk[i] = C.uint8_t(publicKey[1+31-i])
			ownPubkey.pk[32+i] = C.uint8_t(publicKey[33+31-i])
		}
		return nil
	}
}

// lescComputeAndReply computes the LESC DHKey from our private key and the
// peer public key received during pairing, and hands it to the SoftDevice.
func lescComputeAndReply(connHandle C.uint16_t) {
	var raw [65]byte
	raw[0] = 4 // SEC1 uncompressed point
	for i := 0; i < 32; i++ {
		raw[1+i] = byte(peerPubkey.pk[31-i])
		raw[33+i] = byte(peerPubkey.pk[63-i])
	}
	peerKey, err := ecdh.P256().NewPublicKey(raw[:])
	if err != nil {
		// Invalid peer public key (not a point on the curve): abort pairing.
		if debug {
			println("lesc dhkey: invalid peer public key:", err.Error())
		}
		C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_AUTHENTICATION_FAILURE)
		return
	}
	shared, err := lescPrivateKey.ECDH(peerKey)
	if err != nil {
		if debug {
			println("lesc dhkey: ECDH failed:", err.Error())
		}
		C.sd_ble_gap_disconnect(connHandle, C.BLE_HCI_AUTHENTICATION_FAILURE)
		return
	}
	// The shared secret is the X coordinate, big-endian; the SoftDevice wants
	// it little-endian.
	for i := 0; i < 32; i++ {
		lescDHKey.key[i] = C.uint8_t(shared[31-i])
	}
	errCode := C.sd_ble_gap_lesc_dhkey_reply(connHandle, &lescDHKey)
	if debug && errCode != 0 {
		println("lesc dhkey reply failed:", errCode)
	}
}
