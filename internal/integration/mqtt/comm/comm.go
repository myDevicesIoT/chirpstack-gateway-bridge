package comm

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Communication defines the communication interface.
type Communication interface {
	// Init sets the connection information.
	Init(mqtt.Client) error

	// Start begins communication with the server.
	Start(mqtt.MessageHandler) error

	// Stop ends communication with the server.
	Stop() error
}
