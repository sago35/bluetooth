//go:build (softdevice && s132v6) || (softdevice && s140v6) || (softdevice && s140v7)

package bluetooth

// This file implements the event handler for SoftDevices with full support:
// both central and peripheral mode. This includes S132 and S140.

/*
#include "nrf_sdm.h"
#include "nrf_nvic.h"
#include "ble.h"
#include "ble_gap.h"
*/
import "C"

import (
	"unsafe"
)

func dumpEvent(id uint16) {
	print("dumpEvent(")
	print(id)
	print(") ")
	if id >= C.BLE_GAP_EVT_BASE && id <= C.BLE_GAP_EVT_LAST {
		switch id {
		case C.BLE_GAP_EVT_CONNECTED:
			println("ev: BLE_GAP_EVT_CONNECTED")
		case C.BLE_GAP_EVT_DISCONNECTED:
			println("ev: BLE_GAP_EVT_DISCONNECTED")
		case C.BLE_GAP_EVT_CONN_PARAM_UPDATE:
			println("ev: BLE_GAP_EVT_CONN_PARAM_UPDATE")
		case C.BLE_GAP_EVT_SEC_PARAMS_REQUEST:
			println("ev: BLE_GAP_EVT_SEC_PARAMS_REQUEST")
		case C.BLE_GAP_EVT_SEC_INFO_REQUEST:
			println("ev: BLE_GAP_EVT_SEC_INFO_REQUEST")
		case C.BLE_GAP_EVT_PASSKEY_DISPLAY:
			println("ev: BLE_GAP_EVT_PASSKEY_DISPLAY")
		case C.BLE_GAP_EVT_KEY_PRESSED:
			println("ev: BLE_GAP_EVT_KEY_PRESSED")
		case C.BLE_GAP_EVT_AUTH_KEY_REQUEST:
			println("ev: BLE_GAP_EVT_AUTH_KEY_REQUEST")
		case C.BLE_GAP_EVT_LESC_DHKEY_REQUEST:
			println("ev: BLE_GAP_EVT_LESC_DHKEY_REQUEST")
		case C.BLE_GAP_EVT_AUTH_STATUS:
			println("ev: BLE_GAP_EVT_AUTH_STATUS")
		case C.BLE_GAP_EVT_CONN_SEC_UPDATE:
			println("ev: BLE_GAP_EVT_CONN_SEC_UPDATE")
		case C.BLE_GAP_EVT_TIMEOUT:
			println("ev: BLE_GAP_EVT_TIMEOUT")
		case C.BLE_GAP_EVT_RSSI_CHANGED:
			println("ev: BLE_GAP_EVT_RSSI_CHANGED")
		case C.BLE_GAP_EVT_ADV_REPORT:
			println("ev: BLE_GAP_EVT_ADV_REPORT")
		case C.BLE_GAP_EVT_SEC_REQUEST:
			println("ev: BLE_GAP_EVT_SEC_REQUEST")
		case C.BLE_GAP_EVT_CONN_PARAM_UPDATE_REQUEST:
			println("ev: BLE_GAP_EVT_CONN_PARAM_UPDATE_REQUEST")
		case C.BLE_GAP_EVT_SCAN_REQ_REPORT:
			println("ev: BLE_GAP_EVT_SCAN_REQ_REPORT")
		case C.BLE_GAP_EVT_PHY_UPDATE_REQUEST:
			println("ev: BLE_GAP_EVT_PHY_UPDATE_REQUEST")
		case C.BLE_GAP_EVT_PHY_UPDATE:
			println("ev: BLE_GAP_EVT_PHY_UPDATE")
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE_REQUEST:
			println("ev: BLE_GAP_EVT_DATA_LENGTH_UPDATE_REQUEST")
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE:
			println("ev: BLE_GAP_EVT_DATA_LENGTH_UPDATE")
		case C.BLE_GAP_EVT_QOS_CHANNEL_SURVEY_REPORT:
			println("ev: BLE_GAP_EVT_QOS_CHANNEL_SURVEY_REPORT")
		case C.BLE_GAP_EVT_ADV_SET_TERMINATED:
			println("ev: BLE_GAP_EVT_ADV_SET_TERMINATED")
		default:
			println("ev: unknown gap:", id)
		}
	} else if id >= C.BLE_GATTC_EVT_BASE && id <= C.BLE_GATTC_EVT_LAST {
		switch id {
		case C.BLE_GATTC_EVT_PRIM_SRVC_DISC_RSP:
			println("ev: BLE_GATTC_EVT_PRIM_SRVC_DISC_RSP")
		case C.BLE_GATTC_EVT_REL_DISC_RSP:
			println("ev: BLE_GATTC_EVT_REL_DISC_RSP")
		case C.BLE_GATTC_EVT_CHAR_DISC_RSP:
			println("ev: BLE_GATTC_EVT_CHAR_DISC_RSP")
		case C.BLE_GATTC_EVT_DESC_DISC_RSP:
			println("ev: BLE_GATTC_EVT_DESC_DISC_RSP")
		case C.BLE_GATTC_EVT_ATTR_INFO_DISC_RSP:
			println("ev: BLE_GATTC_EVT_ATTR_INFO_DISC_RSP")
		case C.BLE_GATTC_EVT_CHAR_VAL_BY_UUID_READ_RSP:
			println("ev: BLE_GATTC_EVT_CHAR_VAL_BY_UUID_READ_RSP")
		case C.BLE_GATTC_EVT_READ_RSP:
			println("ev: BLE_GATTC_EVT_READ_RSP")
		case C.BLE_GATTC_EVT_CHAR_VALS_READ_RSP:
			println("ev: BLE_GATTC_EVT_CHAR_VALS_READ_RSP")
		case C.BLE_GATTC_EVT_WRITE_RSP:
			println("ev: BLE_GATTC_EVT_WRITE_RSP")
		case C.BLE_GATTC_EVT_HVX:
			println("ev: BLE_GATTC_EVT_HVX")
		case C.BLE_GATTC_EVT_EXCHANGE_MTU_RSP:
			println("ev: BLE_GATTC_EVT_EXCHANGE_MTU_RSP")
		case C.BLE_GATTC_EVT_TIMEOUT:
			println("ev: BLE_GATTC_EVT_TIMEOUT")
		case C.BLE_GATTC_EVT_WRITE_CMD_TX_COMPLETE:
			println("ev: BLE_GATTC_EVT_WRITE_CMD_TX_COMPLETE")
		default:
			println("ev: unknown gattc:", id)
		}
	} else if id >= C.BLE_GATTS_EVT_BASE && id <= C.BLE_GATTS_EVT_LAST {
		switch id {
		case C.BLE_GATTS_EVT_WRITE:
			println("ev: BLE_GATTS_EVT_WRITE")
		case C.BLE_GATTS_EVT_RW_AUTHORIZE_REQUEST:
			println("ev: BLE_GATTS_EVT_RW_AUTHORIZE_REQUEST")
		case C.BLE_GATTS_EVT_SYS_ATTR_MISSING:
			println("ev: BLE_GATTS_EVT_SYS_ATTR_MISSING")
		case C.BLE_GATTS_EVT_HVC:
			println("ev: BLE_GATTS_EVT_HVC")
		case C.BLE_GATTS_EVT_SC_CONFIRM:
			println("ev: BLE_GATTS_EVT_SC_CONFIRM")
		case C.BLE_GATTS_EVT_EXCHANGE_MTU_REQUEST:
			println("ev: BLE_GATTS_EVT_EXCHANGE_MTU_REQUEST")
		case C.BLE_GATTS_EVT_TIMEOUT:
			println("ev: BLE_GATTS_EVT_TIMEOUT")
		case C.BLE_GATTS_EVT_HVN_TX_COMPLETE:
			println("ev: BLE_GATTS_EVT_HVN_TX_COMPLETE")
		default:
			println("ev: unknown gatts:", id)
		}
	} else if id >= C.BLE_L2CAP_EVT_BASE && id <= C.BLE_L2CAP_EVT_LAST {
		switch id {
		case C.BLE_L2CAP_EVT_CH_SETUP_REQUEST:
			println("ev: BLE_L2CAP_EVT_CH_SETUP_REQUEST")
		case C.BLE_L2CAP_EVT_CH_SETUP_REFUSED:
			println("ev: BLE_L2CAP_EVT_CH_SETUP_REFUSE")
		case C.BLE_L2CAP_EVT_CH_SETUP:
			println("ev: BLE_L2CAP_EVT_CH_SETU")
		case C.BLE_L2CAP_EVT_CH_RELEASED:
			println("ev: BLE_L2CAP_EVT_CH_RELEASE")
		case C.BLE_L2CAP_EVT_CH_SDU_BUF_RELEASED:
			println("ev: BLE_L2CAP_EVT_CH_SDU_BUF_RELEASE")
		case C.BLE_L2CAP_EVT_CH_CREDIT:
			println("ev: BLE_L2CAP_EVT_CH_CREDI")
		case C.BLE_L2CAP_EVT_CH_RX:
			println("ev: BLE_L2CAP_EVT_CH_R")
		case C.BLE_L2CAP_EVT_CH_TX:
			println("ev: BLE_L2CAP_EVT_CH_T")
		default:
			println("ev: unknown l2cap:", id)
		}
	} else {
		println("")
	}
}

func handleEvent() {
	id := eventBuf.header.evt_id
	dumpEvent(id)
	switch {
	case id >= C.BLE_GAP_EVT_BASE && id <= C.BLE_GAP_EVT_LAST:
		gapEvent := eventBuf.evt.unionfield_gap_evt()
		switch id {
		case C.BLE_GAP_EVT_CONNECTED: // 16
			connectEvent := gapEvent.params.unionfield_connected()
			device := Device{
				Address:          Address{makeMACAddress(connectEvent.peer_addr)},
				connectionHandle: gapEvent.conn_handle,
			}
			switch connectEvent.role {
			case C.BLE_GAP_ROLE_PERIPH:
				if debug {
					println("evt: connected in peripheral role")
				}
				currentConnection.handle.Reg = uint16(gapEvent.conn_handle)
				DefaultAdapter.connectHandler(device, true)
			case C.BLE_GAP_ROLE_CENTRAL:
				if debug {
					println("evt: connected in central role")
				}
				connectionAttempt.connectionHandle = gapEvent.conn_handle
				connectionAttempt.state.Set(2) // connection was successful
				DefaultAdapter.connectHandler(device, true)
			}
		case C.BLE_GAP_EVT_DISCONNECTED: // 17
			if debug {
				disconn := gapEvent.params.unionfield_disconnected()
				println("evt: disconnected with reason:", disconn.reason)

			}
			// Clean up state for this connection.
			for i, cb := range gattcNotificationCallbacks {
				if uint16(cb.connectionHandle) == currentConnection.handle.Reg {
					gattcNotificationCallbacks[i].valueHandle = 0 // 0 means invalid
				}
			}
			currentConnection.handle.Reg = C.BLE_CONN_HANDLE_INVALID
			// Auto-restart advertisement if needed.
			if defaultAdvertisement.isAdvertising.Get() != 0 {
				// The advertisement was running but was automatically stopped
				// by the connection event.
				// Note that it cannot be restarted during connect like this,
				// because it would need to be reconfigured as a non-connectable
				// advertisement. That's left as a future addition, if
				// necessary.
				C.sd_ble_gap_adv_start(defaultAdvertisement.handle, C.BLE_CONN_CFG_TAG_DEFAULT)
			}
			device := Device{
				connectionHandle: gapEvent.conn_handle,
			}
			DefaultAdapter.connectHandler(device, false)
		case C.BLE_GAP_EVT_CONN_PARAM_UPDATE:
			if debug {
				// Print connection parameters for easy debugging.
				params := gapEvent.params.unionfield_conn_param_update().conn_params
				interval_ms := params.min_conn_interval * 125 / 100 // min and max are the same here
				print("conn param update interval=", interval_ms, "ms latency=", params.slave_latency, " timeout=", params.conn_sup_timeout*10, "ms")
				println()
			}
		case C.BLE_GAP_EVT_ADV_REPORT:
			advReport := gapEvent.params.unionfield_adv_report()
			if debug && &scanReportBuffer.data[0] != (*byte)(unsafe.Pointer(advReport.data.p_data)) {
				// Sanity check.
				panic("scanReportBuffer != advReport.p_data")
			}
			// Prepare the globalScanResult, which will be passed to the
			// callback.
			scanReportBuffer.len = byte(advReport.data.len)
			globalScanResult.RSSI = int16(advReport.rssi)
			globalScanResult.Address = Address{
				makeMACAddress(advReport.peer_addr),
			}
			globalScanResult.AdvertisementPayload = &scanReportBuffer
			// Signal to the main thread that there was a scan report.
			// Scanning will be resumed (from the main thread) once the scan
			// report has been processed.
			gotScanReport.Set(1)
		case C.BLE_GAP_EVT_CONN_PARAM_UPDATE_REQUEST:
			// Respond with the default PPCP connection parameters by passing
			// nil:
			// > If NULL is provided on a peripheral role, the parameters in the
			// > PPCP characteristic of the GAP service will be used instead. If
			// > NULL is provided on a central role and in response to a
			// > BLE_GAP_EVT_CONN_PARAM_UPDATE_REQUEST, the peripheral request
			// > will be rejected
			C.sd_ble_gap_conn_param_update(gapEvent.conn_handle, nil)
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE_REQUEST:
			// We need to respond with sd_ble_gap_data_length_update. Setting
			// both parameters to nil will make sure we send the default values.
			params := gapEvent.params.unionfield_data_length_update_request().peer_params
			_ = params
			C.sd_ble_gap_data_length_update(gapEvent.conn_handle, nil, nil)
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE:
			// ignore confirmation of data length successfully updated
			params := gapEvent.params.unionfield_data_length_update().effective_params
			println("BLE_GAP_EVT_DATA_LENGTH_UPDATE :", params.max_tx_octets, params.max_rx_octets, params.max_tx_time_us, params.max_rx_time_us)
		case C.BLE_GAP_EVT_PHY_UPDATE_REQUEST:
			phyUpdateRequest := gapEvent.params.unionfield_phy_update_request()
			C.sd_ble_gap_phy_update(gapEvent.conn_handle, &phyUpdateRequest.peer_preferred_phys)
		case C.BLE_GAP_EVT_CONN_SEC_UPDATE:
			if debug {
				println("evt: connection security update")
			}
		case C.BLE_GAP_EVT_SEC_INFO_REQUEST:
			if debug {
				println("evt: sec info request")
			}

			secInfo := gapEvent.params.unionfield_sec_info_request()
			var errCode uint32
			if secInfo.master_id.ediv == secKeySet.keys_peer.p_enc_key.master_id.ediv {
				println("evid match")
				secKeySet.keys_peer.p_enc_key.enc_info.set_bitfield_lesc(1)
				errCode = C.sd_ble_gap_sec_info_reply(gapEvent.conn_handle, &secKeySet.keys_peer.p_enc_key.enc_info,
					&secKeySet.keys_peer.p_id_key.id_info, nil) //secKeySet.keys_own
			} else {
				println("evid no match")
				errCode = C.sd_ble_gap_sec_info_reply(gapEvent.conn_handle, nil,
					&secKeySet.keys_peer.p_id_key.id_info, nil)
			}
			if errCode != 0 {
				println("security info request failed:", Error(errCode).Error())
				return
			}
			println("security info request succeesess")
		case C.BLE_GAP_EVT_PASSKEY_DISPLAY:
			params := gapEvent.params.unionfield_passkey_display()
			data := (*[6]byte)(unsafe.Pointer(&params.passkey[0]))[:6:6]
			println(string(data))
		case C.BLE_GAP_EVT_AUTH_STATUS:
			// here we get auth response
			if debug {
				authStatus := gapEvent.params.unionfield_auth_status()
				println("auth_status :",
					authStatus.auth_status,
					authStatus.bitfield_error_src(),
					authStatus.bitfield_bonded(),
					authStatus.bitfield_lesc(),
					authStatus.sm1_levels.bitfield_lv1(),
					authStatus.sm1_levels.bitfield_lv2(),
					authStatus.sm1_levels.bitfield_lv3(),
					authStatus.sm2_levels.bitfield_lv1(),
					authStatus.sm2_levels.bitfield_lv2(),
					authStatus.sm2_levels.bitfield_lv3(),
					authStatus.kdist_own.bitfield_enc(),
					authStatus.kdist_own.bitfield_id(),
					authStatus.kdist_own.bitfield_sign(),
					authStatus.kdist_own.bitfield_link(),
					authStatus.kdist_peer.bitfield_enc(),
					authStatus.kdist_peer.bitfield_id(),
					authStatus.kdist_peer.bitfield_sign(),
					authStatus.kdist_peer.bitfield_link(),
				)
				if authStatus.auth_status != C.BLE_GAP_SEC_STATUS_SUCCESS {
					if authStatus.bitfield_error_src() == C.BLE_GAP_SEC_STATUS_SOURCE_LOCAL {
						println("auth failed from local source")
					} else {
						println("auth failed from remote source")
					}
					println("auth status failed with status:", authStatus.auth_status)
					break
				}
				if authStatus.bitfield_bonded() == 1 {
					println("auth status is bonded")
				}
				if authStatus.bitfield_lesc() == 1 {
					println("connection is LE secure")
				}
				if authStatus.kdist_own.bitfield_enc() != 0 {
					println("local peer distributed encription keys")
				}
				if authStatus.kdist_own.bitfield_id() != 0 {
					println("local peer distributed id keys")
				}
				if authStatus.kdist_own.bitfield_sign() != 0 {
					println("local peer distributed sign keys")
				}
				if authStatus.kdist_own.bitfield_link() != 0 {
					println("local peer distributed link keys")
				}

				if authStatus.kdist_peer.bitfield_enc() != 0 {
					println("remote peer distributed encription keys")
				}
				if authStatus.kdist_peer.bitfield_id() != 0 {
					println("remote peer distributed id keys")
				}
				if authStatus.kdist_peer.bitfield_sign() != 0 {
					println("remote peer distributed sign keys")
				}
				if authStatus.kdist_peer.bitfield_link() != 0 {
					println("remote peer distributed link keys")
				}
			}

			// TODO: save keys to flash for pairing/bonding
		case C.BLE_GAP_EVT_PHY_UPDATE:
			// ignore confirmation of phy successfully updated
		case C.BLE_GAP_EVT_TIMEOUT:
			timeoutEvt := gapEvent.params.unionfield_timeout()
			switch timeoutEvt.src {
			case C.BLE_GAP_TIMEOUT_SRC_CONN:
				// Failed to connect to a peripheral.
				if debug {
					println("gap timeout: conn")
				}
				connectionAttempt.state.Set(3) // connection timed out
			default:
				// For example a scan timeout.
				if debug {
					println("gap timeout: other")
				}
			}
		// ignore confirmation of phy successfully updated
		case C.BLE_GAP_EVT_SEC_PARAMS_REQUEST:
			if debug {
				println("evt: security parameters request")
			}
			// would assume this depends on the role,
			// as for central we need to call sd_ble_gap_authenticate after connection esteblished instead
			params := gapEvent.params.unionfield_sec_params_request().peer_params
			println("bond", params.bitfield_bond())
			println("mitm", params.bitfield_mitm())
			println("lesc", params.bitfield_lesc())
			println("keypress", params.bitfield_keypress())
			println("io_caps", params.bitfield_io_caps())
			println("oob", params.bitfield_oob())
			println("min_ks", params.min_key_size)
			println("max_ks", params.max_key_size)

			// in general key can be null, i would assume in our case we need to read it from flash here
			// so we we do not reapprove bonding
			//SetSecParamsLesc2()
			//SetSecParamsLesc3()
			println("---")
			println("bond", secParams.bitfield_bond())
			println("mitm", secParams.bitfield_mitm())
			println("lesc", secParams.bitfield_lesc())
			println("keypress", secParams.bitfield_keypress())
			println("io_caps", secParams.bitfield_io_caps())
			println("oob", secParams.bitfield_oob())
			println("min_ks", secParams.min_key_size)
			println("max_ks", secParams.max_key_size)
			//secParams.set_bitfield_bond(1)
			//secParams.set_bitfield_mitm(1)
			//secParams.set_bitfield_lesc(1)
			//secParams.set_bitfield_io_caps(uint8(4))
			// /**@defgroup BLE_GAP_IO_CAPS GAP IO Capabilities
			//  * @{ */
			// #define BLE_GAP_IO_CAPS_DISPLAY_ONLY      0x00   /**< Display Only. */
			// #define BLE_GAP_IO_CAPS_DISPLAY_YESNO     0x01   /**< Display and Yes/No entry. */
			// #define BLE_GAP_IO_CAPS_KEYBOARD_ONLY     0x02   /**< Keyboard Only. */
			// #define BLE_GAP_IO_CAPS_NONE              0x03   /**< No I/O capabilities. */
			// #define BLE_GAP_IO_CAPS_KEYBOARD_DISPLAY  0x04   /**< Keyboard and Display. */
			// /**@} */
			errCode := C.sd_ble_gap_sec_params_reply(gapEvent.conn_handle, C.BLE_GAP_SEC_STATUS_SUCCESS, &secParams, &secKeySet)
			if errCode != 0 {
				println("security parameters response failed:", Error(errCode).Error(), errCode)
				return
			}
			if debug {
				println("successfully established security parameters exchange")
			}

		case C.BLE_GAP_EVT_LESC_DHKEY_REQUEST:
			if debug {
				println("evt: lesc dhkey request")
			}
			//	lesc_request := gapEvent.params.unionfield_lesc_dhkey_request()
			//DefaultAdapter.lescRequestHandler(lesc_request.p_pk_peer.pk[:])
			DefaultAdapter.lescRequestHandler(secKeySet.keys_peer.p_pk.pk[:])

		default:
			if debug {
				println("unknown GAP event:", id, id-C.BLE_GAP_EVT_BASE)
			}
		}
	case id >= C.BLE_GATTS_EVT_BASE && id <= C.BLE_GATTS_EVT_LAST:
		gattsEvent := eventBuf.evt.unionfield_gatts_evt()
		switch id {
		case C.BLE_GATTS_EVT_WRITE:
			writeEvent := gattsEvent.params.unionfield_write()
			len := writeEvent.len - writeEvent.offset
			data := (*[255]byte)(unsafe.Pointer(&writeEvent.data[0]))[:len:len]
			for i, x := range data {
				if i > 0 {
					print(",")
				}
				print(x)
			}
			println()
			handler := DefaultAdapter.getCharWriteHandler(writeEvent.handle)
			if handler != nil {
				handler.callback(Connection(gattsEvent.conn_handle), int(writeEvent.offset), data)
			}
		case C.BLE_GATTS_EVT_SYS_ATTR_MISSING:
			// This event is generated when reading the Generic Attribute
			// service. It appears to be necessary for bonded devices.
			// From the docs:
			// > If the pointer is NULL, the system attribute info is
			// > initialized, assuming that the application does not have any
			// > previously saved system attribute data for this device.
			// Maybe we should look at the error, but as there's not really a
			// way to handle it, ignore it.
			C.sd_ble_gatts_sys_attr_set(gattsEvent.conn_handle, nil, 0, 0)
		case C.BLE_GATTS_EVT_EXCHANGE_MTU_REQUEST:
			// This event is generated by some devices. While we could support
			// larger MTUs, this default MTU is supported everywhere.
			C.sd_ble_gatts_exchange_mtu_reply(gattsEvent.conn_handle, C.BLE_GATT_ATT_MTU_DEFAULT)
		case C.BLE_GATTS_EVT_HVN_TX_COMPLETE:
			// ignore confirmation of a notification successfully sent
		default:
			if debug {
				println("unknown GATTS event:", id, id-C.BLE_GATTS_EVT_BASE)
			}
		}
	case id >= C.BLE_GATTC_EVT_BASE && id <= C.BLE_GATTC_EVT_LAST:
		gattcEvent := eventBuf.evt.unionfield_gattc_evt()
		switch id {
		case C.BLE_GATTC_EVT_PRIM_SRVC_DISC_RSP:
			discoveryEvent := gattcEvent.params.unionfield_prim_srvc_disc_rsp()
			if debug {
				println("evt: discovered primary service", discoveryEvent.count)
			}
			discoveringService.state.Set(2) // signal there is a result
			if discoveryEvent.count >= 1 {
				// Theoretically there may be more, but as we're only using
				// sd_ble_gattc_primary_services_discover, there should only be
				// one discovered service. Use the first as a sensible fallback.
				discoveringService.startHandle.Set(discoveryEvent.services[0].handle_range.start_handle)
				discoveringService.endHandle.Set(discoveryEvent.services[0].handle_range.end_handle)
				discoveringService.uuid = discoveryEvent.services[0].uuid
			} else {
				// No service found.
				discoveringService.startHandle.Set(0)
			}
		case C.BLE_GATTC_EVT_CHAR_DISC_RSP:
			discoveryEvent := gattcEvent.params.unionfield_char_disc_rsp()
			if debug {
				println("evt: discovered characteristics", discoveryEvent.count)
			}
			if discoveryEvent.count >= 1 {
				// There may be more, but for ease of implementing we only
				// handle the first.
				discoveringCharacteristic.handle_value.Set(discoveryEvent.chars[0].handle_value)
				discoveringCharacteristic.char_props = discoveryEvent.chars[0].char_props
				discoveringCharacteristic.uuid = discoveryEvent.chars[0].uuid
			} else {
				// zero indicates we received no characteristic, set handle_value to last
				discoveringCharacteristic.handle_value.Set(0xffff)
			}
		case C.BLE_GATTC_EVT_DESC_DISC_RSP:
			discoveryEvent := gattcEvent.params.unionfield_desc_disc_rsp()
			if debug {
				println("evt: discovered descriptors", discoveryEvent.count)
			}
			if discoveryEvent.count >= 1 {
				// There may be more, but for ease of implementing we only
				// handle the first.
				uuid := discoveryEvent.descs[0].uuid
				if uuid._type == C.BLE_UUID_TYPE_BLE && uuid.uuid == 0x2902 {
					// Found a CCCD (Client Characteristic Configuration
					// Descriptor), which has a 16-bit UUID with value 0x2902).
					discoveringCharacteristic.handle_value.Set(discoveryEvent.descs[0].handle)
				} else {
					// Found something else?
					// TODO: handle this properly by continuing the scan. For
					// now, give up if we found something other than a CCCD.
					if debug {
						println("  found some other descriptor (unimplemented)")
					}
				}
			}
		case C.BLE_GATTC_EVT_READ_RSP:
			readEvent := gattcEvent.params.unionfield_read_rsp()
			if debug {
				println("evt: read response, data length", readEvent.len)
			}
			readingCharacteristic.handle_value.Set(readEvent.handle)
			readingCharacteristic.offset = readEvent.offset
			readingCharacteristic.length = readEvent.len

			// copy read event data into Go slice
			copy(readingCharacteristic.value, (*[255]byte)(unsafe.Pointer(&readEvent.data[0]))[:readEvent.len:readEvent.len])
		case C.BLE_GATTC_EVT_HVX:
			hvxEvent := gattcEvent.params.unionfield_hvx()
			switch hvxEvent._type {
			case C.BLE_GATT_HVX_NOTIFICATION:
				if debug {
					println("evt: notification", hvxEvent.handle)
				}
				// Find the callback and call it (if there is any).
				for _, callbackInfo := range gattcNotificationCallbacks {
					if callbackInfo.valueHandle == hvxEvent.handle && callbackInfo.connectionHandle == gattcEvent.conn_handle {
						// Create a Go slice from the data, to pass to the
						// callback.
						data := (*[255]byte)(unsafe.Pointer(&hvxEvent.data[0]))[:hvxEvent.len:hvxEvent.len]
						if callbackInfo.callback != nil {
							callbackInfo.callback(data)
						}
						break
					}
				}
			}
		default:
			if debug {
				println("unknown GATTC event:", id, id-C.BLE_GATTC_EVT_BASE)
			}
		}
	default:
		if debug {
			println("unknown event:", id)
		}
	}
}
