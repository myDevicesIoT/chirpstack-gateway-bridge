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
