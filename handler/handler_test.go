package handler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
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
			  "franciscocardosogil-myTestAllGrouped": [
  				{"franciscocardosogil-myOctetString": "0102030405060708090a0b"},
  				{"franciscocardosogil-myInteger32": -99},
  				{"franciscocardosogil-myInteger64": -99},
  				{"franciscocardosogil-myUnsigned32": 99},
  				{"franciscocardosogil-myUnsigned64": 99},
  				{"franciscocardosogil-myFloat32": 99.9},
  				{"franciscocardosogil-myFloat64": 99.9},
  				{"franciscocardosogil-myAddress": "1.2.3.4"},
  				{"franciscocardosogil-myTime": "1966-11-26T03:34:08 UTC"},
  				{"franciscocardosogil-myString": "Hello, world!"},
  				{"franciscocardosogil-myDiameterIdentity": "Diameter@identity"},
  				{"franciscocardosogil-myDiameterURI": "Diameter@URI"},
  				{"franciscocardosogil-myIPFilterRule": "allow all"},
  				{"franciscocardosogil-myIPv4Address": "4.5.6.7"},
  				{"franciscocardosogil-myIPv6Address": "bebe:cafe::0"},
  				{"franciscocardosogil-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"franciscocardosogil-myEnumerated": "two"}
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

	handler := NewHandler("testServer")
	go handler.Run()

	time.Sleep(200 * time.Millisecond)

	transCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	client := http.Client{Timeout: 2 * time.Second, Transport: transCfg}

	// resp, err := client.Get("https://127.0.0.1:8080/diameterRequest")
	httpResp, err := client.Post("https://127.0.0.1:8080/diameterRequest", "application/json", strings.NewReader(jDiameterMessage))
	if err != nil {
		fmt.Printf("Error %s", err)
		return
	}
	defer httpResp.Body.Close()

	jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("error reading response %s", err)
	}

	// Unserialize to Diameter Message
	var diameterAnswer diamcodec.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		t.Errorf("unmarshal error for diameter message: %s", err)
	}

	fmt.Println(diameterAnswer)
}
