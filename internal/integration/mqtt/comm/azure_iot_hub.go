package comm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-gateway-bridge/internal/config"
)

type message int

const (
	getTwin message = iota
	patchProperties
)

const (
	twinResTopic = "$iothub/twin/res/#"

	twinGetTopic             = "$iothub/twin/GET/?$rid=%s"
	twinPatchPropertiesTopic = "$iothub/twin/PATCH/properties/reported/?$rid=%s"
)

// DesiredProperties represents the Azure Digital Twin desired properties.
type DesiredProperties struct {
	Version int `json:"$version"`
}

// ReportedProperties represents the Azure Digital Twin reported properties.
type ReportedProperties struct {
	SerialNumber string `json:"serialNumber"`
}

// DigitalTwin represents the Azure Digital Twin.
type DigitalTwin struct {
	Desired  DesiredProperties  `json:"desired"`
	Reported ReportedProperties `json:"reported"`
}

func (dt DigitalTwin) String() string {
	return fmt.Sprintf("Desired:%+v, Reported:%+v", dt.Desired, dt.Reported)
}

// AzureIoTHubCommunication implements the Azure IoT Hub communication.
type AzureIoTHubCommunication struct {
	conn         mqtt.Client
	qos          uint8
	deviceID     string
	commandTopic string
	getRequestID string
	twin         DigitalTwin
}

// NewAzureIoTHubCommunication creates an AzureIoTHubCommunication.
func NewAzureIoTHubCommunication(conf config.Config) (Communication, error) {
	handler := AzureIoTHubCommunication{
		qos:      conf.Integration.MQTT.Auth.Generic.QOS,
		deviceID: conf.Integration.MQTT.Auth.AzureIoTHub.DeviceID,
	}

	commandTopicTemplate, err := template.New("event").Parse(conf.Integration.MQTT.CommandTopicTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "mqtt/comm: parse event-topic template error")
	}
	topic := bytes.NewBuffer(nil)
	if err := commandTopicTemplate.Execute(topic, struct{ GatewayID string }{handler.deviceID}); err != nil {
		return nil, errors.Wrap(err, "mqtt/comm: execute command topic template error")
	}
	handler.commandTopic = topic.String()
	return &handler, nil
}

// Init sets the connection information.
func (a *AzureIoTHubCommunication) Init(c mqtt.Client) error {
	a.conn = c
	return nil
}

// Start begins the Azure IoT Hub communication.
func (a *AzureIoTHubCommunication) Start(fallbackHandler mqtt.MessageHandler) error {
	a.subscribe(a.commandTopic, fallbackHandler)
	a.subscribe(twinResTopic, a.handleMessage)
	a.publish(getTwin)
	return nil
}

// Stop ends the Azure IoT Hub communication.
func (a *AzureIoTHubCommunication) Stop() error {
	a.unsubscribe(a.commandTopic)
	a.unsubscribe(twinResTopic)
	return nil
}

func (a *AzureIoTHubCommunication) subscribe(topic string, handler mqtt.MessageHandler) error {
	log.WithFields(log.Fields{
		"topic": topic,
		"qos":   a.qos,
	}).Info("mqtt/comm: subscribing to topic")
	if token := a.conn.Subscribe(topic, a.qos, handler); token.Wait() && token.Error() != nil {
		return errors.Wrap(token.Error(), "subscribe topic error")
	}
	return nil
}

func (a *AzureIoTHubCommunication) unsubscribe(topic string) error {
	log.WithFields(log.Fields{
		"topic": topic,
		"qos":   a.qos,
	}).Info("mqtt/comm: unsubscribing from topic")
	if token := a.conn.Unsubscribe(topic); token.Wait() && token.Error() != nil {
		return errors.Wrap(token.Error(), "subscribe topic error")
	}
	return nil
}

func (a *AzureIoTHubCommunication) publish(msg message) error {
	requestID, err := uuid.NewV4()
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"requestID": requestID,
		}).Error("mqtt/comm: get random request id error")
		return err
	}
	var topic string
	var payload []byte
	switch msg {
	case getTwin:
		a.getRequestID = requestID.String()
		topic = fmt.Sprintf(twinGetTopic, requestID)
		payload = nil
	case patchProperties:
		topic = fmt.Sprintf(twinPatchPropertiesTopic, requestID)
		payload, err = json.Marshal(ReportedProperties{
			SerialNumber: a.deviceID,
		})
		if err != nil {
			return err
		}
	default:
		return errors.New("mqtt/comm: cannot publish unknown message type")
	}
	log.WithFields(log.Fields{
		"topic":   topic,
		"payload": string(payload),
	}).Info("mqtt/comm: publishing message")
	if token := a.conn.Publish(topic, a.qos, false, payload); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (a *AzureIoTHubCommunication) handleMessage(c mqtt.Client, msg mqtt.Message) {
	log.WithFields(log.Fields{
		"topic":   msg.Topic(),
		"payload": string(msg.Payload()),
	}).Info("mqtt/comm: message received")
	parts := strings.SplitN(msg.Topic(), "/", 5)
	statusCode, _ := strconv.Atoi(parts[3])
	log.WithFields(log.Fields{
		"statusCode": statusCode,
	}).Debug("mqtt/comm: incoming message status")
	topic, err := url.Parse(msg.Topic())
	if err != nil {
		log.WithError(err).Error("mqtt/comm: parse error")
	}
	params, _ := url.ParseQuery(topic.RawQuery)

	if params["$rid"][0] == a.getRequestID && statusCode == 200 {
		if err := json.Unmarshal(msg.Payload(), &a.twin); err != nil {
			log.WithError(err).Error("mqtt/comm: error unmarshalling payload")
		}
		log.WithFields(log.Fields{
			"twin": a.twin,
		}).Info("mqtt/comm: digital twin received")
		if a.twin.Reported.SerialNumber != a.deviceID {
			a.publish(patchProperties)
		}
	}
	if statusCode >= 400 {
		log.WithField("statusCode", statusCode).Error("mqtt/comm: error with request")
	}
}
