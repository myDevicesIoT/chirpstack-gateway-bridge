package comm

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-gateway-bridge/internal/config"
)

const (
	twinResTopic = "$iothub/twin/res/#"
	methodsTopic = "$iothub/methods/POST/#"

	twinGetTopic             = "$iothub/twin/GET/?$rid=%s"
	twinPatchPropertiesTopic = "$iothub/twin/PATCH/properties/reported/?$rid=%s"
	methodsResponseTopic     = "$iothub/methods/res/%d/?$rid=%s"
)

// DesiredProperties represents the Azure Digital Twin desired properties.
type DesiredProperties struct {
	Version int `json:"$version"`
}

// ReportedProperties represents the Azure Digital Twin reported properties.
type ReportedProperties struct {
	Make         string `json:"make,omitempty" mapstructure:"make"`
	Model        string `json:"model,omitempty" mapstructure:"model"`
	SerialNumber string `json:"serialNumber,omitempty" mapstructure:"serial_number"`
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
	conn            mqtt.Client
	qos             uint8
	deviceID        string
	commandTopic    string
	twinRequestID   string
	commandID       string
	twin            DigitalTwin
	commandChan     chan<- gw.GatewayCommandExecRequest
	fallbackHandler mqtt.MessageHandler
}

// NewAzureIoTHubCommunication creates an AzureIoTHubCommunication.
func NewAzureIoTHubCommunication(conf config.Config) (Communication, error) {
	a := AzureIoTHubCommunication{
		qos:      conf.Integration.MQTT.Auth.Generic.QOS,
		deviceID: conf.Integration.MQTT.Auth.AzureIoTHub.DeviceID,
	}
	a.twin.Reported.SerialNumber = a.deviceID
	mapstructure.Decode(conf.MetaData.Static, &a.twin.Reported)

	commandTopicTemplate, err := template.New("event").Parse(conf.Integration.MQTT.CommandTopicTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "mqtt/comm: parse event-topic template error")
	}
	topic := bytes.NewBuffer(nil)
	if err := commandTopicTemplate.Execute(topic, struct{ GatewayID string }{a.deviceID}); err != nil {
		return nil, errors.Wrap(err, "mqtt/comm: execute command topic template error")
	}
	a.commandTopic = topic.String()
	return &a, nil
}

// Init sets the connection information.
func (a *AzureIoTHubCommunication) Init(c mqtt.Client, fallbackHandler mqtt.MessageHandler, commandChan chan<- gw.GatewayCommandExecRequest) error {
	a.conn = c
	a.fallbackHandler = fallbackHandler
	a.commandChan = commandChan
	return nil
}

// Start begins the Azure IoT Hub communication.
func (a *AzureIoTHubCommunication) Start() error {
	a.subscribe(a.commandTopic, a.fallbackHandler)
	a.subscribe(twinResTopic, a.handleMessage)
	a.subscribe(methodsTopic, a.handleCommand)
	a.publishGetTwin()
	return nil
}

// PublishEvent publishes event to Azure IoT Hub.
func (a *AzureIoTHubCommunication) PublishEvent(event string, msg proto.Message) error {
	if event == "exec" {
		return a.publishMethodsResponse(msg)
	}
	return nil
}

// Stop ends the Azure IoT Hub communication.
func (a *AzureIoTHubCommunication) Stop() error {
	a.unsubscribe(a.commandTopic)
	a.unsubscribe(twinResTopic)
	a.unsubscribe(methodsTopic)
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

func (a *AzureIoTHubCommunication) newRequestID() string {
	requestID, err := uuid.NewV4()
	if err != nil {
		return "1"
	}
	return requestID.String()
}

func (a *AzureIoTHubCommunication) publish(topic string, payload []byte) error {
	log.WithFields(log.Fields{
		"topic":   topic,
		"payload": string(payload),
	}).Info("mqtt/comm: publishing message")
	if token := a.conn.Publish(topic, a.qos, false, payload); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (a *AzureIoTHubCommunication) publishGetTwin() error {
	a.twinRequestID = a.newRequestID()
	topic := fmt.Sprintf(twinGetTopic, a.twinRequestID)
	return a.publish(topic, nil)
}

func (a *AzureIoTHubCommunication) publishPatchProperties() error {
	topic := fmt.Sprintf(twinPatchPropertiesTopic, a.newRequestID())
	payload, err := json.Marshal(a.twin.Reported)
	if err != nil {
		return err
	}
	return a.publish(topic, payload)
}

func (a *AzureIoTHubCommunication) publishMethodsResponse(msg proto.Message) error {
	statusCode := 200
	response := msg.String()
	if strings.Contains(response, "error") {
		statusCode = 500
	}
	topic := fmt.Sprintf(methodsResponseTopic, statusCode, a.commandID)
	payload, err := json.Marshal(map[string]string{
		"response": response,
	})
	if err != nil {
		return err
	}
	return a.publish(topic, payload)
}

func (a *AzureIoTHubCommunication) parseTopic(msg mqtt.Message) (url.Values, error) {
	topic, err := url.Parse(msg.Topic())
	if err != nil {
		log.WithError(err).Error("mqtt/comm: parse error")
	}
	return url.ParseQuery(topic.RawQuery)
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
	params, _ := a.parseTopic(msg)

	if params["$rid"][0] == a.twinRequestID && statusCode == 200 {
		var receivedTwin DigitalTwin
		if err := json.Unmarshal(msg.Payload(), &receivedTwin); err != nil {
			log.WithError(err).Error("mqtt/comm: error unmarshalling payload")
		}
		log.WithFields(log.Fields{
			"twin": receivedTwin,
		}).Info("mqtt/comm: digital twin received")
		a.twin.Desired = receivedTwin.Desired
		if receivedTwin.Reported != a.twin.Reported {
			if err := a.publishPatchProperties(); err != nil {
				log.WithError(err).Error("mqtt/comm: error publishing properties")
			}
		}
	}
	if statusCode >= 400 {
		log.WithField("statusCode", statusCode).Error("mqtt/comm: error with request")
	}
}

func (a *AzureIoTHubCommunication) handleCommand(c mqtt.Client, msg mqtt.Message) {
	log.WithFields(log.Fields{
		"topic": msg.Topic(),
	}).Info("mqtt/comm: command received")
	log.WithFields(log.Fields{
		"payload": string(msg.Payload()),
	}).Debug("mqtt/comm: command payload")

	parts := strings.SplitN(msg.Topic(), "/", 5)
	params, _ := a.parseTopic(msg)
	a.commandID = params["$rid"][0]

	var gatewayCommandExecRequest gw.GatewayCommandExecRequest
	if err := json.Unmarshal(msg.Payload(), &gatewayCommandExecRequest); err != nil {
		log.WithFields(log.Fields{
			"topic": msg.Topic(),
		}).WithError(err).Error("mqtt/comm: unmarshal gateway command execution request error")
		return
	}
	gatewayCommandExecRequest.GatewayId, _ = hex.DecodeString(a.deviceID)
	gatewayCommandExecRequest.Command = parts[3]
	gatewayCommandExecRequest.ExecId = []byte(params["$rid"][0])

	a.commandChan <- gatewayCommandExecRequest
}
