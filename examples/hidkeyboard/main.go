// Example BLE HID keyboard (HID over GATT profile), currently supported on
// Nordic SoftDevices only.
//
// The device advertises as a keyboard (appearance 961) and pairs with Just
// Works bonding when the host connects. Three buttons (active low, with
// pull-ups) type the letters a, b and c, with proper press/release reports so
// key repeat and chords work. Additionally, any byte typed on the serial
// console is sent to the host as a keystroke - useful on boards without
// buttons wired to the pins below.
//
// The device only bonds with one central at a time. Once bonded, it rejects
// pairing attempts from any other central, so a nearby device cannot take
// over the keyboard. To pair with a new central, hold down the a and b
// buttons during the first few seconds after powering on (or resetting) the
// device: this opens a one-time pairing window that closes again as soon as
// a new device has paired.
//
// On Windows, add the device via Settings > Bluetooth & devices. If it does
// not show up in the scan, set "Bluetooth devices discovery" to "Advanced".
package main

import (
	"machine"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

// Standard keyboard report map: 8 modifier bits, 1 reserved byte, 5 LED
// output bits and 6 key codes (the same report layout as the USB boot
// protocol, without report IDs).
var reportMap = []byte{
	0x05, 0x01, // Usage Page (Generic Desktop)
	0x09, 0x06, // Usage (Keyboard)
	0xA1, 0x01, // Collection (Application)
	0x05, 0x07, //   Usage Page (Key Codes)
	0x19, 0xE0, //   Usage Minimum (224)
	0x29, 0xE7, //   Usage Maximum (231)
	0x15, 0x00, //   Logical Minimum (0)
	0x25, 0x01, //   Logical Maximum (1)
	0x75, 0x01, //   Report Size (1)
	0x95, 0x08, //   Report Count (8)
	0x81, 0x02, //   Input (Data, Variable, Absolute): modifier keys
	0x95, 0x01, //   Report Count (1)
	0x75, 0x08, //   Report Size (8)
	0x81, 0x01, //   Input (Constant): reserved byte
	0x95, 0x05, //   Report Count (5)
	0x75, 0x01, //   Report Size (1)
	0x05, 0x08, //   Usage Page (LEDs)
	0x19, 0x01, //   Usage Minimum (1)
	0x29, 0x05, //   Usage Maximum (5)
	0x91, 0x02, //   Output (Data, Variable, Absolute): LEDs
	0x95, 0x01, //   Report Count (1)
	0x75, 0x03, //   Report Size (3)
	0x91, 0x01, //   Output (Constant): padding
	0x95, 0x06, //   Report Count (6)
	0x75, 0x08, //   Report Size (8)
	0x15, 0x00, //   Logical Minimum (0)
	0x25, 0x65, //   Logical Maximum (101)
	0x05, 0x07, //   Usage Page (Key Codes)
	0x19, 0x00, //   Usage Minimum (0)
	0x29, 0x65, //   Usage Maximum (101)
	0x81, 0x00, //   Input (Data, Array): key codes
	0xC0, // End Collection
}

var inputReport bluetooth.Characteristic

// button is a key switch between a GPIO pin and ground.
type button struct {
	pin     machine.Pin
	char    byte
	pressed bool
}

var buttons = [...]button{
	{pin: machine.P1_06, char: 'a'},
	{pin: machine.P1_04, char: 'b'},
	{pin: machine.P0_22, char: 'c'},
}

func main() {
	// Configure the buttons before anything else, so the pairing-mode
	// gesture below can read them.
	for i := range buttons {
		buttons[i].pin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	}

	// Holding the a and b buttons down during the first moments after
	// power-on (or reset) is the physical gesture that authorizes pairing
	// with a new central. The state is read after this startup delay, not
	// before it, so there is an actual window in which to press them: a
	// flash tool resets the board on its own, giving no way to have a finger
	// on the buttons at the exact moment of reset.
	time.Sleep(3 * time.Second)
	pairingModeRequested := !buttons[0].pin.Get() && !buttons[1].pin.Get()

	println("starting")
	must("enable BLE stack", adapter.Enable())

	// HID hosts expect bonding; Just Works needs no user interaction.
	must("enable pairing", adapter.EnablePairing(bluetooth.PairingParams{
		IOCapabilities: bluetooth.IOCapsNone,
		LESC:           true,
		PairingCompleteHandler: func(device bluetooth.Device, err error) {
			if err != nil {
				println("pairing failed:", err.Error())
			} else {
				println("pairing complete")
			}
		},
	}))
	if pairingModeRequested {
		adapter.AllowNewPairing(true)
		println("pairing mode: a new central may pair until one does")
	}

	// The device information service with the PnP ID characteristic is
	// required by the HID over GATT profile.
	must("add device information service", adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDDeviceInformation,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID:  bluetooth.CharacteristicUUIDManufacturerNameString,
				Value: []byte("TinyGo"),
				Flags: bluetooth.CharacteristicReadPermission,
			},
			{
				UUID: bluetooth.CharacteristicUUIDPnPID,
				// Vendor ID source (0x02 = USB), vendor 0x1915, product
				// 0x0001, version 0x0001 (all little-endian).
				Value: []byte{0x02, 0x15, 0x19, 0x01, 0x00, 0x01, 0x00},
				Flags: bluetooth.CharacteristicReadPermission,
			},
		},
	}))

	// The battery service is also part of the HID over GATT profile.
	must("add battery service", adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDBattery,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID:  bluetooth.CharacteristicUUIDBatteryLevel,
				Value: []byte{100},
				Flags: bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
			},
		},
	}))

	// The HID service itself. Its characteristics may only be accessed over
	// an encrypted link (Just Works pairing is enough).
	must("add HID service", adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDHumanInterfaceDevice,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID: bluetooth.CharacteristicUUIDHIDInformation,
				// bcdHID 1.11, no country code, normally connectable.
				Value: []byte{0x11, 0x01, 0x00, 0x02},
				Flags: bluetooth.CharacteristicReadPermission,
			},
			{
				UUID:         bluetooth.CharacteristicUUIDReportMap,
				Value:        reportMap,
				Flags:        bluetooth.CharacteristicReadPermission,
				ReadSecurity: bluetooth.SecurityEncrypted,
			},
			{
				UUID:         bluetooth.CharacteristicUUIDProtocolMode,
				Value:        []byte{1}, // report protocol
				Flags:        bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				ReadSecurity: bluetooth.SecurityEncrypted,
			},
			{
				Handle:       &inputReport,
				UUID:         bluetooth.CharacteristicUUIDReport,
				Value:        make([]byte, 8),
				Flags:        bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
				ReadSecurity: bluetooth.SecurityEncrypted,
				Descriptors: []bluetooth.DescriptorConfig{
					{
						UUID:         bluetooth.New16BitUUID(0x2908), // Report Reference
						Value:        []byte{0, 1},                   // report ID 0, input report
						ReadSecurity: bluetooth.SecurityEncrypted,
					},
				},
			},
			{
				UUID:          bluetooth.CharacteristicUUIDReport,
				Value:         []byte{0},
				Flags:         bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				ReadSecurity:  bluetooth.SecurityEncrypted,
				WriteSecurity: bluetooth.SecurityEncrypted,
				WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
					if len(value) > 0 {
						println("keyboard LED state:", value[0])
					}
				},
				Descriptors: []bluetooth.DescriptorConfig{
					{
						UUID:         bluetooth.New16BitUUID(0x2908), // Report Reference
						Value:        []byte{0, 2},                   // report ID 0, output report
						ReadSecurity: bluetooth.SecurityEncrypted,
					},
				},
			},
			{
				UUID:          bluetooth.CharacteristicUUIDHIDControlPoint,
				Value:         []byte{0},
				Flags:         bluetooth.CharacteristicWriteWithoutResponsePermission,
				WriteSecurity: bluetooth.SecurityEncrypted,
			},
		},
	}))

	adv := adapter.DefaultAdvertisement()
	must("config adv", adv.Configure(bluetooth.AdvertisementOptions{
		LocalName:    "TinyGo Keyboard",
		ServiceUUIDs: []bluetooth.UUID{bluetooth.ServiceUUIDHumanInterfaceDevice},
		Appearance:   961, // keyboard
	}))
	must("start adv", adv.Start())
	println("advertising as 'TinyGo Keyboard'...")
	println("press a button or type on the serial console to send keystrokes")

	for {
		changed := false
		for i := range buttons {
			pressed := !buttons[i].pin.Get() // active low
			if pressed != buttons[i].pressed {
				buttons[i].pressed = pressed
				changed = true
			}
		}
		if changed {
			sendButtonReport()
		}
		if b, err := machine.Serial.ReadByte(); err == nil {
			typeKey(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sendButtonReport sends a report with all currently pressed buttons, so that
// pressing and releasing each button behaves like a real key (including key
// repeat by the host and pressing multiple keys at once).
func sendButtonReport() {
	report := make([]byte, 8)
	n := 2 // key codes start at byte 2
	for i := range buttons {
		if buttons[i].pressed && n < len(report) {
			_, report[n] = asciiToKeycode(buttons[i].char)
			n++
		}
	}
	inputReport.Write(report)
}

// typeKey sends a key press and release for an ASCII character.
func typeKey(char byte) {
	modifier, code := asciiToKeycode(char)
	if code == 0 {
		return
	}
	inputReport.Write([]byte{modifier, 0, code, 0, 0, 0, 0, 0})
	inputReport.Write(make([]byte, 8))
}

const shift = 0x02 // left shift modifier bit

// asciiToKeycode converts an ASCII character to a HID modifier and key code
// (US keyboard layout). It returns a zero key code for unsupported
// characters.
func asciiToKeycode(char byte) (modifier, code byte) {
	switch {
	case char >= 'a' && char <= 'z':
		return 0, char - 'a' + 4
	case char >= 'A' && char <= 'Z':
		return shift, char - 'A' + 4
	case char >= '1' && char <= '9':
		return 0, char - '1' + 30
	case char == '0':
		return 0, 39
	case char == '\r' || char == '\n':
		return 0, 40 // enter
	case char == 0x08 || char == 0x7f:
		return 0, 42 // backspace
	case char == '\t':
		return 0, 43 // tab
	case char == ' ':
		return 0, 44
	}
	// Punctuation on the US layout, in pairs of unshifted/shifted characters
	// per key code starting at 45 (the "-=[]\;'`,./" keys).
	const punct = "-_=+[{]}\\|;:'\"`~,<.>/?"
	for i := 0; i < len(punct); i += 2 {
		code := byte(45 + i/2)
		if code >= 50 {
			code++ // skip 50, the non-US # key
		}
		if char == punct[i] {
			return 0, code
		}
		if char == punct[i+1] {
			return shift, code
		}
	}
	// Shifted digits: !@#$%^&*()
	const shiftedDigits = "!@#$%^&*()"
	for i := 0; i < len(shiftedDigits); i++ {
		if char == shiftedDigits[i] {
			if i == 9 {
				return shift, 39 // ')' is on the 0 key
			}
			return shift, byte(30 + i)
		}
	}
	return 0, 0
}

func must(action string, err error) {
	if err != nil {
		panic("failed to " + action + ": " + err.Error())
	}
}
