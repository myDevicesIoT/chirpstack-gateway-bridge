package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/brocaar/chirpstack-gateway-bridge/internal/config"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	registrationsResponseTopic = "$dps/registrations/res/#"

	registrationsTopic = "$dps/registrations/PUT/iotdps-register/?$rid=%s"
	operationsTopic    = "$dps/registrations/GET/iotdps-get-operationstatus/?$rid=%s&operationId=%s"

	registrationsEndpoint = "https://%s/%s/registrations/%s/register"
	operationsEndpoint    = "https://%s/%s/registrations/%s/operations/%s"
)

// ProvisioningClient connects to the provisioning server and provisions devices.
type ProvisioningClient struct {
	mqttClient            MQTT.Client
	httpClient            http.Client
	opts                  Options
	mqttOpts              *MQTT.ClientOptions
	messageChan           chan message
	registrationStateChan chan RegistrationState
	requestScheduled      bool
}

// Options contains the device info used for provisioning.
type Options struct {
	Endpoint       string
	Scope          string
	RegistrationID string
	Cert           string
	Key            string
	OutputFile     string
	Protocol       string
}

// RegistrationRequest represents the body of a DPS registration request
type RegistrationRequest struct {
	RegistrationID string `json:"registrationId"`
}

// RegistrationState represents the body of a DPS registration response containing the current state of a device
type RegistrationState struct {
	RegistrationID         string `json:"registrationId"`
	CreatedDateTimeUtc     string `json:"createdDateTimeUtc"`
	AssignedHub            string `json:"assignedHub"`
	DeviceID               string `json:"deviceId"`
	Status                 string `json:"status"`
	SubStatus              string `json:"substatus"`
	LastUpdatedDateTimeUtc string `json:"lastUpdatedDateTimeUtc"`
	ETag                   string `json:"etag"`
}

// RegistrationResponse represents the body of a DPS registration response
type RegistrationResponse struct {
	OperationID       string            `json:"operationId"`
	Status            string            `json:"status"`
	RegistrationState RegistrationState `json:"registrationState"`
}

type message struct {
	params               url.Values
	registrationResponse RegistrationResponse
	statusCode           int
}

// NewAzureIoTHubProvisioning creates a new provisioning client.
func NewAzureIoTHubProvisioning(conf config.Config) (*ProvisioningClient, error) {
	azureConf := conf.Integration.MQTT.Auth.AzureIoTHub
	opts := Options{
		Endpoint:       azureConf.ProvisioningEndpoint,
		Scope:          azureConf.ProvisioningScope,
		RegistrationID: azureConf.DeviceID,
		Cert:           azureConf.TLSCert,
		Key:            azureConf.TLSKey,
	}
	if opts.Endpoint == "" || opts.Scope == "" || opts.RegistrationID == "" || opts.Cert == "" || opts.Key == "" {
		log.WithFields(log.Fields{
			"options": opts,
		}).Info("azure/dps: missing option")
		return nil, errors.New("missing option")
	}
	log.Info("azure/dps: creating client")
	c := ProvisioningClient{
		opts:                  opts,
		messageChan:           make(chan message),
		registrationStateChan: make(chan RegistrationState),
		requestScheduled:      false,
	}
	tlsconfig := c.newTLSConfig()
	if strings.Contains(opts.Protocol, "http") {
		transport := &http.Transport{TLSClientConfig: tlsconfig}
		c.httpClient = http.Client{Transport: transport}
	} else {
		// MQTT.CRITICAL = log.StandardLogger()
		// MQTT.ERROR = log.StandardLogger()
		// MQTT.WARN = log.StandardLogger()
		// MQTT.DEBUG = log.StandardLogger()
		c.mqttOpts = MQTT.NewClientOptions()
		server := fmt.Sprintf("ssl://%s:8883", c.opts.Endpoint)
		username := fmt.Sprintf("%s/registrations/%s/api-version=2019-03-31", c.opts.Scope, c.opts.RegistrationID)
		c.mqttOpts.AddBroker(server)
		c.mqttOpts.SetClientID(c.opts.RegistrationID)
		c.mqttOpts.SetUsername(username)
		c.mqttOpts.SetTLSConfig(tlsconfig)
		c.mqttOpts.SetConnectRetry(true)
		c.mqttOpts.SetAutoReconnect(true)
		c.mqttOpts.SetOnConnectHandler(func(client MQTT.Client) {
			log.Trace("azure/dps: connected")
			log.WithField("topic", registrationsResponseTopic).Info("azure/dps: subscribing to topic")
			go client.Subscribe(registrationsResponseTopic, 1, nil)
			go c.sendRegisterRequest(0)
		})
		c.mqttOpts.SetConnectionLostHandler(func(client MQTT.Client, err error) {
			log.Debug("azure/dps: connection lost")
		})
		// c.mqttOpts.SetReconnectingHandler(func(client MQTT.Client, opts *MQTT.ClientOptions) {
		// 	log.Debug("azure/dps: reconnecting")
		// })
		c.mqttOpts.SetDefaultPublishHandler(c.messageHandler)

		c.mqttClient = MQTT.NewClient(c.mqttOpts)
	}
	return &c, nil
}

// ProvisionDevice connects to the server and provisions the device.
func (c *ProvisioningClient) ProvisionDevice() (*RegistrationState, error) {
	err := c.connect()
	if err != nil {
		log.Fatal("azure/dps: connection error")
		return nil, err
	}

	go c.messageLoop()
	registrationState := <-c.registrationStateChan

	log.WithFields(log.Fields{
		"hub":       registrationState.AssignedHub,
		"device id": registrationState.DeviceID,
	}).Info("azure/dps: registered device")

	c.writeConfigFile(registrationState)

	if c.mqttClient != nil {
		log.Info("azure/dps: disconnecting")
		c.mqttClient.Disconnect(250)
	}
	return &registrationState, nil
}

func (c *ProvisioningClient) newTLSConfig() *tls.Config {
	certpool := x509.NewCertPool()
	rootCAs := fmt.Sprintf("%s%s%s", digiCertBaltimoreRootCA, microsoftRSARootCA2017, digiCertGlobalRootG2)
	if !certpool.AppendCertsFromPEM([]byte(rootCAs)) {
		log.Fatal("azure/dps: append ca certs from pem error")
	}

	cert, err := tls.LoadX509KeyPair(c.opts.Cert, c.opts.Key)
	if err != nil {
		log.Fatal(err)
	}

	return &tls.Config{
		RootCAs:      certpool,
		Certificates: []tls.Certificate{cert},
	}
}

func (c *ProvisioningClient) connect() error {
	if c.mqttClient != nil {
		log.Info("azure/dps: connecting")
		token := c.mqttClient.Connect()
		token.Wait()
		return token.Error()
	}
	go c.sendRegisterRequest(0)
	return nil
}

func (c *ProvisioningClient) messageLoop() {
	delay := int64(10)
	retryAfter := int64(2)
	for true {
		select {
		case msg := <-c.messageChan:
			if _, hasParam := msg.params["retry-after"]; hasParam {
				retryAfter, _ = strconv.ParseInt(msg.params["retry-after"][0], 10, 64)
			}
			switch {
			case msg.statusCode >= 300:
				log.WithFields(log.Fields{
					"statusCode": msg.statusCode,
				}).Error("azure/dps: incoming message failure")
				if msg.statusCode <= 429 {
					const maxDelay = 1800
					delay += 10
					if delay > maxDelay {
						delay = maxDelay
					}
					retryAfter = delay
				}
				go func(msg message) {
					log.Infof("azure/dps: retry register after %v seconds", retryAfter)
					go c.sendRegisterRequest(time.Duration(retryAfter) * time.Second)
				}(msg)
			default:
				switch msg.registrationResponse.Status {
				case "assigning":
					go c.sendOperationStatusRequest(time.Duration(retryAfter)*time.Second, msg)
				case "assigned":
					c.registrationStateChan <- msg.registrationResponse.RegistrationState
					return
				}
			}
		case <-time.After((time.Duration(retryAfter) + 30) * time.Second):
			log.Error("azure/dps: timed out, retrying request")
			go c.sendRegisterRequest(0)
		}
	}
}

func (c *ProvisioningClient) newRequestID() string {
	requestID, err := uuid.NewV4()
	if err != nil {
		return "1"
	}
	return requestID.String()
}

func (c *ProvisioningClient) sendRegisterRequest(delay time.Duration) {
	if c.requestScheduled {
		return
	}
	c.requestScheduled = true
	if delay > 0 {
		time.Sleep(delay)
	}

	body, err := json.Marshal(RegistrationRequest{
		RegistrationID: c.opts.RegistrationID,
	})
	if err != nil {
		return
	}
	if c.mqttClient != nil {
		topic := fmt.Sprintf(registrationsTopic, c.newRequestID())
		log.WithFields(log.Fields{
			"topic":   topic,
			"payload": string(body),
		}).Info("azure/dps: sending register message")
		c.mqttClient.Publish(topic, 1, false, body)
	} else {
		url := fmt.Sprintf(registrationsEndpoint, c.opts.Endpoint, c.opts.Scope, c.opts.RegistrationID)
		log.WithFields(log.Fields{
			"url": url,
		}).Info("azure/dps: sending register request")
		c.sendHTTPRequest(http.MethodPut, url, body)
	}
	c.requestScheduled = false
}

func (c *ProvisioningClient) sendOperationStatusRequest(delay time.Duration, msg message) {
	if delay > 0 {
		time.Sleep(delay)
	}
	if c.mqttClient != nil {
		topic := fmt.Sprintf(operationsTopic, c.newRequestID(), msg.registrationResponse.OperationID)
		log.WithFields(log.Fields{
			"topic": topic,
		}).Info("azure/dps: sending operation status message")
		c.mqttClient.Publish(topic, 1, false, " ")
	} else {
		url := fmt.Sprintf(operationsEndpoint, c.opts.Endpoint, c.opts.Scope, c.opts.RegistrationID, msg.registrationResponse.OperationID)
		log.WithFields(log.Fields{
			"url": url,
		}).Info("azure/dps: sending operation status request")
		c.sendHTTPRequest(http.MethodGet, url, nil)
	}
}

func (c *ProvisioningClient) sendHTTPRequest(method, url string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*30))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add("api-version", "2019-03-31")
	req.URL.RawQuery = q.Encode()
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	receivedMsg := &message{
		statusCode: resp.StatusCode,
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.WithField("body", string(respBody)).Info("azure/dps: response")
	json.Unmarshal(respBody, &receivedMsg.registrationResponse)
	if err != nil {
		return err
	}
	// bodyDec := json.NewDecoder(resp.Body)
	// err = bodyDec.Decode(&receivedMsg.registrationResponse)
	// if err != nil && err != io.EOF {
	// 	return err
	// }
	c.messageChan <- *receivedMsg
	return nil
}

func (c *ProvisioningClient) messageHandler(client MQTT.Client, msg MQTT.Message) {
	const processingError = 600
	log.WithFields(log.Fields{
		"topic":   msg.Topic(),
		"payload": string(msg.Payload()),
	}).Info("azure/dps: received message")
	parts := strings.SplitN(msg.Topic(), "/", 5)
	statusCode, err := strconv.Atoi(parts[3])
	if err != nil {
		statusCode = processingError
	}
	log.WithFields(log.Fields{
		"statusCode": statusCode,
	}).Debug("azure/dps: incoming message status")
	paramString := strings.SplitN(parts[4], "$", -1)
	params, err := url.ParseQuery(paramString[1])
	if err != nil {
		statusCode = processingError
	}
	receivedMsg := message{
		params: params,
	}
	if err := json.Unmarshal(msg.Payload(), &receivedMsg.registrationResponse); err != nil {
		statusCode = processingError
	}
	receivedMsg.statusCode = statusCode
	c.messageChan <- receivedMsg
}

func (c *ProvisioningClient) writeConfigFile(registrationState RegistrationState) {
	if config.C.Integration.MQTT.Auth.AzureIoTHub.Hostname != registrationState.AssignedHub {
		log.WithFields(log.Fields{
			"hub": registrationState.AssignedHub,
		}).Info("azure/dps: writing updated config")
		config.C.Integration.MQTT.Auth.AzureIoTHub.Hostname = registrationState.AssignedHub
		b, err := ioutil.ReadFile(viper.ConfigFileUsed())
		outputConfig := viper.New()
		if err == nil {
			outputConfig.SetConfigType("toml")
			outputConfig.ReadConfig(bytes.NewBuffer(b))
			outputConfig.SetConfigFile(viper.ConfigFileUsed())
		} else {
			log.WithError(err).Error("azure/dps: error reading current config file")
		}
		outputConfig.Set("integration.mqtt.auth.azure_iot_hub.hostname", registrationState.AssignedHub)
		if err := outputConfig.WriteConfig(); err != nil {
			log.WithError(err).Error("azure/dps: error writing config file")
		}
	}
}
