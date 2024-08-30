//go:build (softdevice && s113v7) || (softdevice && s132v6) || (softdevice && s140v6) || (softdevice && s140v7)

package bluetooth

// This file defines the SoftDevice adapter for all nrf52-series chips.

/*
#include "nrf_sdm.h"
#include "nrf_nvic.h"
#include "ble.h"
#include "ble_gap.h"

void assertHandler(void);

ble_gap_sec_params_t secParamsX = {
    .min_key_size = 7,
    .max_key_size = 16,
};
*/
import "C"

import (
	"machine"
	"unsafe"
)

// TODO: Probably it should be in adapter_sd, but as it's usage is added only for nrf528xx-full.go
// as well as i do not have other machines to test, adding it here for now

type GapIOCapability uint8

const (
	DisplayOnlyGapIOCapability     = C.BLE_GAP_IO_CAPS_DISPLAY_ONLY
	DisplayYesNoGapIOCapability    = C.BLE_GAP_IO_CAPS_DISPLAY_YESNO
	KeyboardOnlyGapIOCapability    = C.BLE_GAP_IO_CAPS_KEYBOARD_ONLY
	NoneGapIOCapability            = C.BLE_GAP_IO_CAPS_NONE
	KeyboardDisplayGapIOCapability = C.BLE_GAP_IO_CAPS_KEYBOARD_DISPLAY
)

var (
	secParams = C.ble_gap_sec_params_t{
		min_key_size: 7, // not sure if those are the best default length
		max_key_size: 16,
	}
	secParamsX = C.secParamsX

	secKeySet C.ble_gap_sec_keyset_t
)

// are those should be methods for adapter as they are relevant for sd only
func SetSecParamsBonding() {
	secParams.set_bitfield_bond(1)
}

func SetSecParamsLesc() {
	secParams.set_bitfield_bond(0) //1)
	secParams.set_bitfield_lesc(1)
	//secParams.set_bitfield_mitm(1)
	//secParams.set_bitfield_io_caps(uint8(DisplayOnlyGapIOCapability))
}

func SetSecParamsLesc2() {
	secParams = secParamsX
	//secParams.set_bitfield_bond(1) //1)
	//secParams.set_bitfield_lesc(1)
	//secParams.set_bitfield_mitm(1)
	//secParams.set_bitfield_io_caps(2) // 3: NONE
}

func SetSecParamsLesc3() {
	secParams.set_bitfield_bond(1) //1)
	secParams.set_bitfield_mitm(1)
	secParams.set_bitfield_lesc(1)
	secParams.set_bitfield_keypress(0)
	//secParams.set_bitfield_io_caps(DisplayOnlyGapIOCapability) // Android 側で数字入力する必要あり (その数字は？
	//secParams.set_bitfield_io_caps(DisplayYesNoGapIOCapability)
	//secParams.set_bitfield_io_caps(KeyboardOnlyGapIOCapability)
	secParams.set_bitfield_io_caps(NoneGapIOCapability)
	//secParams.set_bitfield_io_caps(KeyboardDisplayGapIOCapability) // Android に表示された数字をキー入力する必要あり
	secParams.set_bitfield_oob(1)
}

func SetLesPublicKey(key []uint8) {
	var pk [C.BLE_GAP_LESC_P256_PK_LEN]uint8
	copy(pk[:], key)
	secKeySet.keys_peer = C.ble_gap_sec_keys_t{
		p_pk:       &C.ble_gap_lesc_p256_pk_t{},
		p_enc_key:  &C.ble_gap_enc_key_t{},
		p_id_key:   &C.ble_gap_id_key_t{},
		p_sign_key: &C.ble_gap_sign_info_t{},
	}
	secKeySet.keys_own = C.ble_gap_sec_keys_t{
		p_enc_key:  &C.ble_gap_enc_key_t{},
		p_id_key:   &C.ble_gap_id_key_t{},
		p_sign_key: &C.ble_gap_sign_info_t{},
		p_pk: &C.ble_gap_lesc_p256_pk_t{
			pk: pk,
		},
	}
}

func ReplyLesc(key []byte) error {
	var k [C.BLE_GAP_LESC_DHKEY_LEN]uint8
	copy(k[:], key)
	lescKey := C.ble_gap_lesc_dhkey_t{
		key: k,
	}
	errCode := C.sd_ble_gap_lesc_dhkey_reply(currentConnection.handle.Reg, &lescKey)
	if errCode != 0 {
		return Error(errCode)

	}
	return nil
}

func SetSecCapabilities(cap GapIOCapability) {
	secParams.set_bitfield_io_caps(uint8(cap))
}

//export assertHandler
func assertHandler() {
	println("SoftDevice assert")
}

var clockConfigXtal C.nrf_clock_lf_cfg_t = C.nrf_clock_lf_cfg_t{
	source:       C.NRF_CLOCK_LF_SRC_XTAL,
	rc_ctiv:      0,
	rc_temp_ctiv: 0,
	accuracy:     C.NRF_CLOCK_LF_ACCURACY_250_PPM,
}

//go:extern __app_ram_base
var appRAMBase [0]uint32

func (a *Adapter) enable() error {
	// Enable the SoftDevice.
	var clockConfig *C.nrf_clock_lf_cfg_t
	if machine.HasLowFrequencyCrystal {
		clockConfig = &clockConfigXtal
	}
	errCode := C.sd_softdevice_enable(clockConfig, C.nrf_fault_handler_t(C.assertHandler))
	if errCode != 0 {
		return Error(errCode)
	}

	// Enable the BLE stack.
	appRAMBase := C.uint32_t(uintptr(unsafe.Pointer(&appRAMBase)))
	errCode = C.sd_ble_enable(&appRAMBase)
	return makeError(errCode)
}

func (a *Adapter) Address() (MACAddress, error) {
	var addr C.ble_gap_addr_t
	errCode := C.sd_ble_gap_addr_get(&addr)
	if errCode != 0 {
		return MACAddress{}, Error(errCode)
	}
	return MACAddress{MAC: makeAddress(addr.addr)}, nil
}

// Convert a C.ble_gap_addr_t to a MACAddress struct.
func makeMACAddress(addr C.ble_gap_addr_t) MACAddress {
	return MACAddress{
		MAC:      makeAddress(addr.addr),
		isRandom: addr.bitfield_addr_type() != 0,
	}
}

func (a *Adapter) SetLescRequestHandler(c func(pubKey []uint8)) {
	a.lescRequestHandler = c
}
