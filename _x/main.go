package main

import (
	"crypto/ecdh"
	"crypto/rand"
	_ "embed"
	"log"
	"machine"
	"time"

	keyboard "github.com/sago35/tinygo-keyboard"
	"tinygo.org/x/bluetooth"
)

type keyEvent struct {
	layer, indx int
	state       keyboard.State
}

func main() {
	machine.InitSerial()
	var err error

	//niceview.ClearScreen()
	err = connect()
	if err != nil {
		println("failed to establish LESC connection:", err.Error())
		log.Fatal(err)
	}
	println("esteblished LESC connection")
	registerHID()
	println("registered HID")
	for {
		time.Sleep(1 * time.Second)
	}
}

func connect() error {
	peerPKey := make([]byte, 0, 64)
	privLesc, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	lescChan := make(chan struct{})
	bluetooth.SetSecParamsLesc()
	bluetooth.SetSecCapabilities(bluetooth.NoneGapIOCapability)
	time.Sleep(4 * time.Second)
	println("getting own pub key")
	var key []byte

	// pk := privLesc.PublicKey().Bytes()
	// pubKey := swapEndinan(pk[1:])
	bluetooth.SetLesPublicKey(swapEndinan(privLesc.PublicKey().Bytes()[1:]))
	// pubKey = nil
	println(" key is set")

	println("register lesc callback")
	adapter.SetLescRequestHandler(
		func(pubKey []byte) {
			println("lescRequestHandler")
			peerPKey = pubKey
			close(lescChan)
		},
	)
	//defer func() {
	//	adapter.SetLescRequestHandler(func(pubkey []byte) {
	//		println("lescRequestHandler 2nd")
	//	})
	//}()
	println("enabling adapter")
	err = adapter.Enable()
	if err != nil {
		return err
	}
	println("def adv")
	adv := adapter.DefaultAdvertisement()
	println("adv config")
	adv.Configure(bluetooth.AdvertisementOptions{
		LocalName: "tinygo-corne",
		ServiceUUIDs: []bluetooth.UUID{
			bluetooth.ServiceUUIDDeviceInformation,
			bluetooth.ServiceUUIDBattery,
			bluetooth.ServiceUUIDHumanInterfaceDevice,
		},
	})
	println("adv start")
	adv.Start()

	select {
	case <-lescChan:
		peerPKey = append([]byte{0x04}, swapEndinan(peerPKey)...)
		p, err := ecdh.P256().NewPublicKey(peerPKey)
		if err != nil {
			println("failed on parsing pub:", err.Error())
			return err
		}
		println("calculating ecdh")
		key, err = privLesc.ECDH(p)
		if err != nil {
			println("failed on curving:", err.Error())
			return err
		}
		println("key len:", len(key))
		return bluetooth.ReplyLesc(swapEndinan(key))
	}

}

func swapEndinan(in []byte) []byte {
	var reverse = make([]byte, len(in))
	for i, b := range in[:32] {

		reverse[31-i] = b
	}
	if len(in) > 32 {
		for i, b := range in[32:] {
			reverse[63-i] = b
		}
	}

	return reverse
}

func registerHID() {
	adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDDeviceInformation,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID:  bluetooth.CharacteristicUUIDManufacturerNameString,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: []byte("Nice Keyboards"),
			},
			{
				UUID:  bluetooth.CharacteristicUUIDModelNumberString,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: []byte("nice!nano"),
			},
			{
				UUID:  bluetooth.CharacteristicUUIDPnPID,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: []byte{0x02, 0x8a, 0x24, 0x66, 0x82, 0x34, 0x36},
				//Value: []byte{0x02, uint8(0x10C4 >> 8), uint8(0x10C4 & 0xff), uint8(0x0001 >> 8), uint8(0x0001 & 0xff)},
			},
		},
	})
	adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDBattery,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID:  bluetooth.CharacteristicUUIDBatteryLevel,
				Value: []byte{80},
				Flags: bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
			},
		},
	})
	// gacc
	/*
	   device name r
	   apperance r
	   peripheral prefreed connection

	*/

	adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDGenericAccess,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				UUID:  bluetooth.CharacteristicUUIDDeviceName,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: []byte("tinygo-corne"),
			},
			{

				UUID:  bluetooth.New16BitUUID(0x2A01),
				Flags: bluetooth.CharacteristicReadPermission,
				Value: []byte{uint8(0x03c4 >> 8), uint8(0x03c4 & 0xff)}, /// []byte(strconv.Itoa(961)),
			},
			// {
			// 	UUID:  bluetooth.CharacteristicUUIDPeripheralPreferredConnectionParameters,
			// 	Flags: bluetooth.CharacteristicReadPermission,
			// 	Value: []byte{0x02},
			// },

			// // 		//
		},
	})

	//v := []byte{0x85, 0x02} // 0x85, 0x02
	reportValue := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	//var reportmap bluetooth.Characteristic

	// hid
	adapter.AddService(&bluetooth.Service{
		UUID: bluetooth.ServiceUUIDHumanInterfaceDevice,
		/*
			 - hid information r
			 - report map r
			 - report nr
			   - client charecteristic configuration
			   - report reference
			- report nr
			   - client charecteristic configuration
			   - report reference
			- hid control point wnr
		*/
		Characteristics: []bluetooth.CharacteristicConfig{
			// {
			// 	UUID:  bluetooth.CharacteristicUUIDHIDInformation,
			// 	Flags: bluetooth.CharacteristicReadPermission,
			// 	Value: []byte{uint8(0x0111 >> 8), uint8(0x0111 & 0xff), uint8(0x0002 >> 8), uint8(0x0002 & 0xff)},
			// },
			{
				//Handle: &reportmap,
				UUID:  bluetooth.CharacteristicUUIDReportMap,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: reportMap,
			},
			{

				Handle: &reportIn,
				UUID:   bluetooth.CharacteristicUUIDReport,
				Value:  reportValue[:],
				Flags:  bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
			},
			{
				// protocl mode
				UUID:  bluetooth.New16BitUUID(0x2A4E),
				Flags: bluetooth.CharacteristicWriteWithoutResponsePermission | bluetooth.CharacteristicReadPermission,
				// Value: []byte{uint8(1)},
				// WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
				// 	print("protocol mode")
				// },
			},
			{
				UUID:  bluetooth.CharacteristicUUIDHIDControlPoint,
				Flags: bluetooth.CharacteristicWriteWithoutResponsePermission,
				//	Value: []byte{0x02},
			},
		},
	})
}
