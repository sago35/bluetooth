module github.com/funkycode/bluetooth/_x

go 1.21.4

require (
	github.com/sago35/tinygo-keyboard v0.0.0-20231108131318-c40abe2ac19a
	tinygo.org/x/bluetooth v0.8.0
)

require (
	github.com/bgould/tinygo-rotary-encoder v0.0.0-20231106003644-94bb14d88946 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/muka/go-bluetooth v0.0.0-20221213043340-85dc80edc4e1 // indirect
	github.com/saltosystems/winrt-go v0.0.0-20230921082907-2ab5b7d431e1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tinygo-org/cbgo v0.0.4 // indirect
	golang.org/x/sys v0.11.0 // indirect
	tinygo.org/x/drivers v0.25.0 // indirect
)

replace tinygo.org/x/bluetooth => ../
