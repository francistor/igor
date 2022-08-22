package httphandler

import (
	"crypto/tls"
	"encoding/json"
	"igor/config"
	"igor/diamcodec"
	"igor/handlerfunctions"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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

func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testServer"
	config.InitHandlerConfigInstance(bootstrapFile, instanceName, true)

	// TODO: Needed to generate answers with origin diameter server name
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, false)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestBasicHandler(t *testing.T) {

	handler := NewHttpHandler("testServer", handlerfunctions.EmptyDiameterHandler, handlerfunctions.EmptyRadiusHandler)
	go handler.Run()

	time.Sleep(200 * time.Millisecond)

	transCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	client := http.Client{Timeout: 2 * time.Second, Transport: transCfg}

	// TODO: Replace this by the helper function

	// resp, err := client.Get("https://127.0.0.1:8080/diameterRequest")
	httpResp, err := client.Post("https://127.0.0.1:8080/diameterRequest", "application/json", strings.NewReader(jDiameterMessage))
	if err != nil {
		t.Fatalf("Error posting request %s", err)
	}
	defer httpResp.Body.Close()

	jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("error reading response %s", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("got status code %d", httpResp.StatusCode)
	}

	// Unserialize to Diameter Message
	var diameterAnswer diamcodec.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		t.Errorf("unmarshal error for diameter message: %s", err)
	}
}
