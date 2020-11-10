package comm

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/protobuf/proto"
)

// Communication defines the communication interface.
type Communication interface {
	// Init sets the connection information.
	Init(c mqtt.Client, fallbackHandler mqtt.MessageHandler, commandHandler mqtt.MessageHandler) error

	// Start begins communication with the server.
	Start() error

	// PublishEvent publish events to the server.
	PublishEvent(event string, msg proto.Message) error

	// Stop ends communication with the server.
	Stop() error
}

// StubCommunication defines a stub communication interface.
type StubCommunication struct {
}

// Init does nothing.
func (StubCommunication) Init(c mqtt.Client, fallbackHandler mqtt.MessageHandler, commandHandler mqtt.MessageHandler) error {
	return nil
}

// Start does nothing.
func (StubCommunication) Start() error {
	return nil
}

// PublishEvent does nothing.
func (StubCommunication) PublishEvent(event string, msg proto.Message) error {
	return nil
}

// Stop does nothing.
func (StubCommunication) Stop() error {
	return nil
}
