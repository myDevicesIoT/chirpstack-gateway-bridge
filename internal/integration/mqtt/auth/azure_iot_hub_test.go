package auth

import (
	"errors"
	"testing"

	"github.com/brocaar/chirpstack-gateway-bridge/internal/config"
	"github.com/brocaar/lorawan"
	"github.com/stretchr/testify/require"
)

func TestAzureIoTHubAuthenticationGetGatewayID(t *testing.T) {
	gatewayID := lorawan.EUI64{0, 128, 0, 0, 160, 0, 22, 182} // 00800000a00016b6

	tests := []struct {
		Name              string
		DeviceID          string
		ExpectedGatewayID *lorawan.EUI64
	}{
		{
			Name:              "device id is a valid EUI64",
			DeviceID:          "00800000a00016b6",
			ExpectedGatewayID: &gatewayID,
		},
		{
			Name:              "device id is not an EUI64",
			DeviceID:          "not-an-eui64",
			ExpectedGatewayID: nil,
		},
		{
			Name:              "device id is empty",
			DeviceID:          "",
			ExpectedGatewayID: nil,
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			var conf config.Config
			conf.Integration.MQTT.Auth.AzureIoTHub.DeviceID = tst.DeviceID
			conf.Integration.MQTT.Auth.AzureIoTHub.Hostname = "gateways-eu868.azure-devices.net"
			conf.Integration.MQTT.Auth.AzureIoTHub.DeviceKey = "WWVQv+auegGaG2mm2/0FIS24xqkmZW/z5cYBO898+8I="

			auth, err := NewAzureIoTHubAuthentication(conf)
			assert.NoError(err)

			assert.Equal(tst.ExpectedGatewayID, auth.GetGatewayID())
		})
	}
}

func TestParseConnectionString(t *testing.T) {
	tests := []struct {
		Name             string
		ConnectionString string
		ExpectedKV       map[string]string
		ExpectedError    error
	}{
		{
			Name:             "valid string",
			ConnectionString: "HostName=gateways-eu868.azure-devices.net;DeviceId=00800000a00016b6;SharedAccessKey=WWVQv+auegGaG2mm2/0FIS24xqkmZW/z5cYBO898+8I=",
			ExpectedKV: map[string]string{
				"HostName":        "gateways-eu868.azure-devices.net",
				"DeviceId":        "00800000a00016b6",
				"SharedAccessKey": "WWVQv+auegGaG2mm2/0FIS24xqkmZW/z5cYBO898+8I=",
			},
		},
		{
			Name:             "invalid string",
			ConnectionString: "HostName;gateways-eu868.azure-devices.net;DeviceId=00800000a00016b6;SharedAccessKey=WWVQv+auegGaG2mm2/0FIS24xqkmZW/z5cYBO898+8I=",
			ExpectedError:    errors.New("expected two items in: [HostName]"),
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			kv, err := parseConnectionString(tst.ConnectionString)
			assert.Equal(tst.ExpectedError, err)
			if err != nil {
				return
			}

			assert.EqualValues(tst.ExpectedKV, kv)
		})
	}
}
