package bluetooth

// Set this to true to print debug messages, for example for unknown events.
const debug = true

// SetConnectHandler sets a handler function to be called whenever the adaptor connects
// or disconnects. You must call this before you call adaptor.Connect() for centrals
// or adaptor.Start() for peripherals in order for it to work.
func (a *Adapter) SetConnectHandler(c func(device Address, connected bool)) {
	a.connectHandler = c
}
