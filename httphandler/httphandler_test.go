package httphandler

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/diamcodec"
	"github.com/francistor/igor/handlerfunctions"
	"github.com/francistor/igor/radiuscodec"

	"golang.org/x/net/http2"
)

var jDiameterMessage = `
	{
		"IsRequest": true,
		"IsProxyable": false,
		"IsError": false,
		"IsRetransmission": false,
		"CommandCode": 2000,
		"ApplicationId": 1000,
		"avps":[
			{
			  "Igor-myTestAllGrouped": [
  				{"Igor-myOctetString": "0102030405060708090a0b"},
  				{"Igor-myInteger32": -99},
  				{"Igor-myInteger64": -99},
  				{"Igor-myUnsigned32": 99},
  				{"Igor-myUnsigned64": 99},
  				{"Igor-myFloat32": 99.9},
  				{"Igor-myFloat64": 99.9},
  				{"Igor-myAddress": "1.2.3.4"},
  				{"Igor-myTime": "1966-11-26T03:34:08 UTC"},
  				{"Igor-myString": "Hello, world!"},
  				{"Igor-myDiameterIdentity": "Diameter@identity"},
  				{"Igor-myDiameterURI": "Diameter@URI"},
  				{"Igor-myIPFilterRule": "allow all"},
  				{"Igor-myIPv4Address": "4.5.6.7"},
  				{"Igor-myIPv6Address": "bebe:cafe::0"},
  				{"Igor-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"Igor-myEnumerated": "two"}
			  ]
			}
		]
	}
	`

var jRadiusRequest = `
	{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-TaggedStringAttribute": "myString:1"},
			{"Igor-SaltedOctetsAttribute": "00"},
			{"User-Name":"MyUserName"}
		]
	}
	`

func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testServer"
	config.InitHttpHandlerConfigInstance(bootstrapFile, instanceName, true)

	// TODO: Needed to generate answers with origin diameter server name
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, false)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestBasicHandlers(t *testing.T) {

	handler := NewHttpHandler("testServer", handlerfunctions.EmptyDiameterHandler, handlerfunctions.EmptyRadiusHandler)

	time.Sleep(200 * time.Millisecond)

	transCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	client := http.Client{Timeout: 2 * time.Second, Transport: transCfg}

	// Diameter request
	httpResp, err := client.Post("https://127.0.0.1:8080/diameterRequest", "application/json", strings.NewReader(jDiameterMessage))
	if err != nil {
		t.Fatalf("Error posting diameter request %s", err)
	}
	defer httpResp.Body.Close()

	jsonAnswer, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("error reading diameter response %s", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("got status code %d", httpResp.StatusCode)
	}

	var diameterAnswer diamcodec.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		t.Fatalf("unmarshal error for diameter message: %s", err)
	}
	if diameterAnswer.GetIntAVP("Result-Code") != diamcodec.DIAMETER_SUCCESS {
		t.Fatalf("answer was not DIAMETER_SUCCESS")
	}

	// Radius Request
	httpResp, err = client.Post("https://127.0.0.1:8080/radiusRequest", "application/json", strings.NewReader(jRadiusRequest))
	if err != nil {
		t.Fatalf("Error posting radius request %s", err)
	}
	defer httpResp.Body.Close()

	jsonAnswer, err = io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("error reading radius response %s", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("got status code %d", httpResp.StatusCode)
	}

	var radiusResponse radiuscodec.RadiusPacket
	err = json.Unmarshal(jsonAnswer, &radiusResponse)
	if err != nil {
		t.Fatalf("unmarshal error for radius message: %s", err)
	}
	if radiusResponse.Code != radiuscodec.ACCESS_ACCEPT {
		t.Log(radiusResponse)
		t.Fatalf("response code was not ACCESS_ACCEPT")
	}

	handler.Close()
}
