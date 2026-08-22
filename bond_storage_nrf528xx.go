//go:build (softdevice && s113v7) || (softdevice && s132v6) || (softdevice && s140v6) || (softdevice && s140v7)

package bluetooth

/*
#include "nrf_soc.h"
*/
import "C"

import (
	"errors"
	"machine"
	"runtime/volatile"
	"time"
	"unsafe"
)

// Bond storage uses the last flash page of the application's flash area (as
// reported by the linker, so it never collides with program code) to persist
// the bond slots across resets: per slot, the LTK plus the CCCD state
// (subscriptions) of the bonded central. A bonded central expects both to
// survive a reset of this device, and does not necessarily subscribe to
// notifications again on reconnection. The page is erased and rewritten as a
// whole every time any slot changes.
//
// Page layout (all fields 4-byte aligned):
//
//	magic (uint32)
//	BondSlotCount slot records of bondSlotRecordSize() bytes each:
//	  valid (uint32, 1 = valid), encKey, idValid (uint32), idKey,
//	  sysAttrLen (uint32), sysAttr
const (
	bondFlashPageSize = 4096
	bondFlashMagicV1  = 0xB0FFEE01 // single-bond layout, read for migration
	bondFlashMagicV2  = 0xB0FFEE02 // five slots without identity keys, read for migration
	bondFlashMagic    = 0xB0FFEE03 // arbitrary, just not the erased-flash value (0xFFFFFFFF)
)

var errFlashOpFailed = errors.New("bluetooth: flash operation failed")

func bondFlashAddr() uintptr {
	return machine.FlashDataEnd() - bondFlashPageSize
}

// bondEncKeyLen and bondIDKeyLen are the stored (4-byte padded) sizes of the
// two key structures, so every uint32 field of a record stays aligned.
func bondEncKeyLen() int {
	return (int(unsafe.Sizeof(ownEncKey)) + 3) &^ 3
}

func bondIDKeyLen() int {
	return (int(unsafe.Sizeof(bondPeerID)) + 3) &^ 3
}

func bondSlotRecordSize() int {
	return 4 + bondEncKeyLen() + 4 + bondIDKeyLen() + 4 + ((len(sysAttrBuf) + 3) &^ 3)
}

func bondSlotRecordSizeV2() int {
	return (4 + int(unsafe.Sizeof(ownEncKey)) + 4 + len(sysAttrBuf) + 3) &^ 3
}

// readBondRecord fills a slot from the record at addr (past its valid flag):
// encKey, idValid, idKey, sysAttrLen, sysAttr. It marks the slot valid.
func readBondRecord(s *bondSlot, addr uintptr) {
	rawEncLen := int(unsafe.Sizeof(s.encKey))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&s.encKey)), rawEncLen), unsafe.Slice((*byte)(unsafe.Pointer(addr)), rawEncLen))
	addr += uintptr(bondEncKeyLen())
	s.idValid = *(*uint32)(unsafe.Pointer(addr)) == 1
	addr += 4
	rawIDLen := int(unsafe.Sizeof(s.idKey))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&s.idKey)), rawIDLen), unsafe.Slice((*byte)(unsafe.Pointer(addr)), rawIDLen))
	addr += uintptr(bondIDKeyLen())
	length := *(*uint32)(unsafe.Pointer(addr))
	copy(s.sysAttr[:], unsafe.Slice((*byte)(unsafe.Pointer(addr+4)), len(s.sysAttr)))
	if length <= uint32(len(s.sysAttr)) {
		s.sysAttrLen = uint16(length)
	}
	s.valid = true
}

// readBondRecordLegacy fills a slot from the identity-less
// encKey/sysAttrLen/sysAttr sequence shared by a v2 slot record (after its
// valid flag) and the single v1 record (after its magic), and marks it valid.
func readBondRecordLegacy(s *bondSlot, addr uintptr) {
	encKeyLen := int(unsafe.Sizeof(s.encKey))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&s.encKey)), encKeyLen), unsafe.Slice((*byte)(unsafe.Pointer(addr)), encKeyLen))
	length := *(*uint32)(unsafe.Pointer(addr + uintptr(encKeyLen)))
	copy(s.sysAttr[:], unsafe.Slice((*byte)(unsafe.Pointer(addr+uintptr(encKeyLen)+4)), len(s.sysAttr)))
	if length <= uint32(len(s.sysAttr)) {
		s.sysAttrLen = uint16(length)
	}
	s.valid = true
}

// loadBondFromFlash restores the persisted bond slots, if any: per slot the
// LTK (so a bonded central can re-encrypt the link via a
// BLE_GAP_EVT_SEC_INFO_REQUEST) and the CCCD state (so its notification
// subscriptions still apply). The active slot is loaded into the live
// single-bond state. It is called once, from EnablePairing, before the
// device has any connection and before the workers start, so it may write
// all bond state directly.
func loadBondFromFlash() {
	addr := bondFlashAddr()
	switch *(*uint32)(unsafe.Pointer(addr)) {
	case bondFlashMagic:
		recordSize := bondSlotRecordSize()
		for i := range bondSlots {
			slotAddr := addr + 4 + uintptr(i*recordSize)
			if *(*uint32)(unsafe.Pointer(slotAddr)) == 1 {
				readBondRecord(&bondSlots[i], slotAddr+4)
			}
		}
	case bondFlashMagicV2:
		// Slot records written before identity keys existed: migrate them
		// without one (their advertising stays unfiltered until the central
		// pairs again). The page is rewritten in the new layout on the next
		// save.
		recordSize := bondSlotRecordSizeV2()
		for i := range bondSlots {
			slotAddr := addr + 4 + uintptr(i*recordSize)
			if *(*uint32)(unsafe.Pointer(slotAddr)) == 1 {
				readBondRecordLegacy(&bondSlots[i], slotAddr+4)
			}
		}
	case bondFlashMagicV1:
		// A record written by the old single-bond layout: migrate it into
		// slot 0, also without an identity key.
		readBondRecordLegacy(&bondSlots[0], addr+4)
	}

	loadStateFromSlot(int(activeBondSlot.Get()))

	// An empty active slot must accept pairing out of the box (fresh flash,
	// or the device restarted while an unused slot was selected): with the
	// window closed it could serve nobody. Bonded slots stay protected: the
	// window closes as soon as a bond is formed.
	if !bondSlots[activeBondSlot.Get()].valid {
		allowNewPairing.Set(1)
	}
}

// saveBondToFlash persists all bond slots, after syncing the active slot
// from the live single-bond state (the LTK in ownEncKey and the CCCD state
// in sysAttrBuf). It must only be called from the bond storage worker (not
// interrupt context) and blocks for the duration of the flash operations
// (tens of milliseconds).
func saveBondToFlash() error {
	syncActiveSlotFromState()
	return writeBondPage()
}

// writeBondPage erases the bond flash page and writes the slot cache back to
// it. Worker context only, like saveBondToFlash.
func writeBondPage() error {
	addr := bondFlashAddr()

	// Serialize with SDFlash operations: completion is reported through the
	// single flashOpResult flag.
	flashOpMu.Lock()
	defer flashOpMu.Unlock()

	flashOpResult.Set(0)
	errCode := C.sd_flash_page_erase(C.uint32_t(uint32(addr) / bondFlashPageSize))
	if errCode != 0 {
		return makeError(errCode)
	}
	if !waitFlashOp() {
		return errFlashOpFailed
	}

	recordSize := bondSlotRecordSize()
	buf := make([]byte, 4+len(bondSlots)*recordSize)
	*(*uint32)(unsafe.Pointer(&buf[0])) = bondFlashMagic
	for i := range bondSlots {
		s := &bondSlots[i]
		record := buf[4+i*recordSize:]
		if !s.valid {
			continue // the valid word stays 0
		}
		*(*uint32)(unsafe.Pointer(&record[0])) = 1
		offset := 4
		rawEncLen := int(unsafe.Sizeof(s.encKey))
		copy(record[offset:offset+rawEncLen], unsafe.Slice((*byte)(unsafe.Pointer(&s.encKey)), rawEncLen))
		offset += bondEncKeyLen()
		if s.idValid {
			*(*uint32)(unsafe.Pointer(&record[offset])) = 1
		}
		offset += 4
		rawIDLen := int(unsafe.Sizeof(s.idKey))
		copy(record[offset:offset+rawIDLen], unsafe.Slice((*byte)(unsafe.Pointer(&s.idKey)), rawIDLen))
		offset += bondIDKeyLen()
		*(*uint32)(unsafe.Pointer(&record[offset])) = uint32(s.sysAttrLen)
		offset += 4
		copy(record[offset:offset+len(s.sysAttr)], s.sysAttr[:])
	}

	flashOpResult.Set(0)
	errCode = C.sd_flash_write((*C.uint32_t)(unsafe.Pointer(addr)), (*C.uint32_t)(unsafe.Pointer(&buf[0])), C.uint32_t(len(buf)/4))
	if errCode != 0 {
		return makeError(errCode)
	}
	if !waitFlashOp() {
		return errFlashOpFailed
	}
	return nil
}

// eraseBondFromFlash invalidates the active slot and rewrites the page;
// bonds in other slots survive. It must only be called from the bond storage
// worker (see bondStorageWorker), so that flash operations never run
// concurrently, except before the workers exist (see RemoveBond).
func eraseBondFromFlash() error {
	bondSlots[activeBondSlot.Get()] = bondSlot{}
	return writeBondPage()
}

// performBondSwitch persists the outgoing active slot and loads slot n in
// its place. It runs on the bond storage worker (or inline before the
// workers exist), with no connection present - SelectBondSlot disconnected
// and waited - so the live single-bond state cannot change concurrently.
func performBondSwitch(n int) error {
	if debug {
		println("bond slot switch:", activeBondSlot.Get(), "->", n, "bonded:", bondSlots[n].valid)
	}
	syncActiveSlotFromState()
	// Everything the dirty flag stands for is persisted right here.
	bondDirty.Set(0)
	err := writeBondPage()
	loadStateFromSlot(n)
	activeBondSlot.Set(uint8(n))
	peerBonded.Set(0)
	// Selecting an empty slot is the explicit "pair a new device here"
	// action (profiles behave like ZMK's), so open the pairing window: the
	// new central can pair right away, without a separate unpair step.
	// Selecting a bonded slot closes the window again so a nearby device
	// cannot silently take over that bond.
	if bondSlots[n].valid {
		allowNewPairing.Set(0)
	} else {
		allowNewPairing.Set(1)
	}
	return err
}

// Erase-request handshake between RemoveBond (application goroutine context)
// and the bond storage worker, in the same volatile-flag style used for all
// other cross-goroutine signalling in this backend. RemoveBond clears
// bondEraseDone, sets bondEraseRequested and then polls bondEraseDone;
// bondEraseErr is only valid once bondEraseDone is set. Only one erase can
// be outstanding at a time (RemoveBond blocks until completion).
var (
	bondEraseRequested volatile.Register8
	bondEraseDone      volatile.Register8
	bondEraseErr       error
)

// bondStorageWorker runs all bond flash operations: deferred saves whenever
// the RAM copy of the bond has changed (bondDirty, set from interrupt
// context) and erase requests from RemoveBond. Routing every flash operation
// through this one goroutine serializes access to the flash and to
// flashOpResult, and keeps the security worker responsive to pairing events
// while a flash operation (up to tens of milliseconds) is in progress.
func bondStorageWorker() {
	for {
		if bondSwitchRequested.Get() != 0 {
			bondSwitchRequested.Set(0)
			bondSwitchErr = performBondSwitch(int(bondSwitchTarget.Get()))
			bondSwitchDone.Set(1)
			continue
		}
		if bondEraseRequested.Get() != 0 {
			bondEraseRequested.Set(0)
			bondEraseErr = eraseBondFromFlash()
			bondEraseDone.Set(1)
			continue
		}
		if bondDirty.Get() != 0 {
			bondDirty.Set(0)
			if err := saveBondToFlash(); debug && err != nil {
				println("failed to persist bond:", err.Error())
			}
			continue
		}
		time.Sleep(16 * time.Millisecond)
	}
}

// waitFlashOp blocks until the SWI2 interrupt handler records the result of
// the flash operation started just before calling this function, and reports
// whether it succeeded. It must be called from thread context (the bond
// storage worker), never from an interrupt. The timeout leaves ample room
// for the slowest flash operation (a page erase takes up to ~90ms, and the
// SoftDevice may delay it to protect radio timing).
func waitFlashOp() bool {
	for i := 0; i < 200; i++ {
		switch flashOpResult.Get() {
		case 1:
			return true
		case 2:
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return false // timed out: the SoftDevice should always raise one of the two events
}
