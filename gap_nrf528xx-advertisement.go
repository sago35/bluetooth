//go:build (softdevice && s113v7) || (softdevice && s132v6) || (softdevice && s140v6) || (softdevice && s140v7)

package bluetooth

import (
	"runtime/volatile"
	"time"
	"unsafe"
)

/*
#include "ble_gap.h"
*/
import "C"

// Address contains a Bluetooth MAC address.
type Address struct {
	MACAddress
}

// Advertisement encapsulates a single advertisement instance.
type Advertisement struct {
	handle        C.uint8_t
	advType       C.uint8_t
	isAdvertising volatile.Register8
	interval      C.uint32_t
	payload       rawAdvertisementPayload
}

// The nrf528xx devices only seem to support one advertisement instance. The way
// multiple advertisements are implemented is by changing the packet data
// frequently.
var defaultAdvertisement = Advertisement{
	handle: C.BLE_GAP_ADV_SET_HANDLE_NOT_SET,
}

// DefaultAdvertisement returns the default advertisement instance but does not
// configure it.
func (a *Adapter) DefaultAdvertisement() *Advertisement {
	return &defaultAdvertisement
}

// Configure this advertisement.
func (a *Advertisement) Configure(options AdvertisementOptions) error {
	// Fill empty options with reasonable defaults.
	if options.Interval == 0 {
		// Pick an advertisement interval recommended by Apple (section 35.5
		// Advertising Interval):
		// https://developer.apple.com/accessories/Accessory-Design-Guidelines.pdf
		options.Interval = NewDuration(152500 * time.Microsecond) // 152.5ms
	}

	if options.Appearance != 0 {
		// Also expose the appearance through the GAP service. Hosts use it to
		// recognize device types, for example HID keyboards.
		if err := makeError(C.sd_ble_gap_appearance_set(C.uint16_t(options.Appearance))); err != nil {
			return err
		}
	}

	// Construct payload.
	// Note that the payload needs to be part of the Advertisement object as the
	// memory is still used after sd_ble_gap_adv_set_configure returns.
	a.payload.reset()
	if !a.payload.addFromOptions(options) {
		return errAdvertisementPacketTooBig
	}

	var typ uint8
	switch options.AdvertisementType {
	case AdvertisingTypeInd:
		typ = C.BLE_GAP_ADV_TYPE_CONNECTABLE_SCANNABLE_UNDIRECTED
	case AdvertisingTypeDirectInd:
		typ = C.BLE_GAP_ADV_TYPE_CONNECTABLE_NONSCANNABLE_DIRECTED
	case AdvertisingTypeScanInd:
		typ = C.BLE_GAP_ADV_TYPE_NONCONNECTABLE_SCANNABLE_UNDIRECTED
	case AdvertisingTypeNonConnInd:
		typ = C.BLE_GAP_ADV_TYPE_NONCONNECTABLE_NONSCANNABLE_UNDIRECTED
	}

	a.advType = C.uint8_t(typ)
	a.interval = C.uint32_t(options.Interval)
	return a.configureSet(C.BLE_GAP_ADV_FP_ANY)
}

// configureSet (re)configures the advertising set from the stored options
// with the given whitelist filter policy. Advertising must be stopped.
func (a *Advertisement) configureSet(filterPolicy C.uint8_t) error {
	data := C.ble_gap_adv_data_t{}
	data.adv_data = C.ble_data_t{
		p_data: (*C.uint8_t)(unsafe.Pointer(&a.payload.data[0])),
		len:    C.uint16_t(a.payload.len),
	}
	params := C.ble_gap_adv_params_t{
		properties: C.ble_gap_adv_properties_t{
			_type: a.advType,
		},
		interval:      a.interval,
		filter_policy: filterPolicy,
	}
	errCode := C.sd_ble_gap_adv_set_configure(&a.handle, &data, &params)
	return makeError(errCode)
}

// Static storage for the advertising filter: it is also applied from the
// advertising auto-restart in the SoftDevice event handler (interrupt
// context), which must not allocate.
var (
	advFilterIDKey   C.ble_gap_id_key_t
	advFilterIDPtr   [1]*C.ble_gap_id_key_t
	advFilterAddr    C.ble_gap_addr_t
	advFilterAddrPtr [1]*C.ble_gap_addr_t
	advFilterActive  bool
)

// applyIdentityFilter points the advertising whitelist at the bonded central
// of the active bond slot, so nobody else can even connect (a central bonded
// to another slot would otherwise connect, fail encryption with a MIC error
// and disconnect, over and over - keeping the single connection slot busy
// and teaching some hosts to stop reconnecting altogether). The whitelist
// opens up whenever it cannot or must not discriminate: no bond, no identity
// key stored with it, or the pairing window is open (a new central has to be
// able to connect to pair).
//
// Advertising must be stopped while this runs: the SoftDevice rejects
// whitelist changes while the whitelist is in use.
func (a *Advertisement) applyIdentityFilter() {
	filter := pairingEnabled.Get() != 0 && bondValid.Get() != 0 &&
		bondPeerIDValid.Get() != 0 && allowNewPairing.Get() == 0
	if !filter && !advFilterActive {
		return // stay away from setups that never used the whitelist
	}
	policy := C.uint8_t(C.BLE_GAP_ADV_FP_ANY)
	if filter {
		advFilterIDKey = bondPeerID
		advFilterAddr = advFilterIDKey.id_addr_info
		advFilterAddrPtr[0] = &advFilterAddr
		// An all-zero IRK cannot resolve anything and is rejected by the
		// SoftDevice; the identity address alone still whitelists a central
		// with a public or static address.
		irkZero := true
		for _, b := range advFilterIDKey.id_info.irk {
			if b != 0 {
				irkZero = false
				break
			}
		}
		if irkZero {
			C.sd_ble_gap_device_identities_set(nil, nil, 0)
		} else {
			advFilterIDPtr[0] = &advFilterIDKey
			C.sd_ble_gap_device_identities_set(&advFilterIDPtr[0], nil, 1)
		}
		C.sd_ble_gap_whitelist_set(&advFilterAddrPtr[0], 1)
		policy = C.BLE_GAP_ADV_FP_FILTER_CONNREQ
	} else {
		C.sd_ble_gap_device_identities_set(nil, nil, 0)
		C.sd_ble_gap_whitelist_set(nil, 0)
	}
	advFilterActive = filter
	if debug {
		println("adv filter: bonded central only:", filter)
	}
	a.configureSet(policy)
}

// reapplyAdvFilter restarts a running advertisement so applyIdentityFilter
// picks up a changed bond state (slot switch, unpair, pairing window). While
// not advertising it does nothing: the next Start applies the state anyway.
func reapplyAdvFilter() {
	a := &defaultAdvertisement
	if a.isAdvertising.Get() == 0 ||
		currentConnection.Get() != C.BLE_CONN_HANDLE_INVALID {
		// Not advertising, or a connection suppressed it (restarting it
		// here would open a second connection): the auto-restart after the
		// disconnect re-derives the filter anyway.
		return
	}
	C.sd_ble_gap_adv_stop(a.handle)
	a.applyIdentityFilter()
	C.sd_ble_gap_adv_start(a.handle, C.BLE_CONN_CFG_TAG_DEFAULT)
}

// Start advertisement. May only be called after it has been configured.
func (a *Advertisement) Start() error {
	a.applyIdentityFilter()
	a.isAdvertising.Set(1)
	errCode := C.sd_ble_gap_adv_start(a.handle, C.BLE_CONN_CFG_TAG_DEFAULT)
	return makeError(errCode)
}

// Stop advertisement.
func (a *Advertisement) Stop() error {
	a.isAdvertising.Set(0)
	errCode := C.sd_ble_gap_adv_stop(a.handle)
	return makeError(errCode)
}

// SetRandomAddress sets the random address to be used for advertising.
func (a *Adapter) SetRandomAddress(mac MAC) error {
	var addr C.ble_gap_addr_t
	addr.addr = makeSDAddress(mac)
	addr.set_bitfield_addr_type(C.BLE_GAP_ADDR_TYPE_RANDOM_STATIC)

	errCode := C.sd_ble_gap_addr_set(&addr)
	if errCode != 0 {
		return Error(errCode)
	}
	return nil
}
