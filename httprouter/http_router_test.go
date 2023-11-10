package httprouter

import (
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/router"

	"golang.org/x/net/http2"
)

// This message handler parses the Igor1-Command, which may specify
// whether to introduce a small delay (value "Slow") or a big one (value "VerySlow")
// A User-Name attribute with the value "TestUserNameEcho" is added to the answer
func diameterHandler(request *core.DiameterMessage) (*core.DiameterMessage, error) {
	answer := core.NewDiameterAnswer(request)
	answer.Add("User-Name", "EchoLocal")
	answer.Add("Result-Code", core.DIAMETER_SUCCESS)

	command := request.GetStringAVP("Igor-Command")
	switch command {
	case "Slow":
		// Simulate the answer takes some time
		time.Sleep(300 * time.Millisecond)
	case "VerySlow":
		// Simulate the answer takes more time
		time.Sleep(5000 * time.Millisecond)
	}

	return answer, nil
}

// The most basic handler ever. Returns an empty response to the received message
func radiusHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {
	resp := core.NewRadiusResponse(request, true)
	resp.Add("User-Name", "EchoLocal")

	command := request.GetStringAVP("Igor-Command")
	switch command {
	case "Slow":
		// Simulate the answer takes some time
		time.Sleep(300 * time.Millisecond)
	case "VerySlow":
		// Simulate the answer takes more time
		time.Sleep(5000 * time.Millisecond)
	}

	return resp, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"

	// Initialize policy
	core.InitPolicyConfigInstance(bootstrapFile, "testServer", nil, embed.FS{}, true)
	core.InitPolicyConfigInstance(bootstrapFile, "testSuperServer", nil, embed.FS{}, false)

	// Execute the tests and exit
	exitCode := m.Run()
	core.IS.Close()
	os.Exit(exitCode)
}

// Requests are sent using http and forwarded to super-servers both for radius and diameter
func TestHttpRouterHandler(t *testing.T) {

	rrouter := router.NewRadiusRouter("testServer", nil).Start()
	drouter := router.NewDiameterRouter("testServer", nil).Start()
	rsserver := router.NewRadiusRouter("testSuperServer", radiusHandler).Start()
	dsserver := router.NewDiameterRouter("testSuperServer", diameterHandler).Start()

	httpRouter := NewHttpRouter("testServer", drouter, rrouter)

	// Get the base url for requests
	httpRouterURL := fmt.Sprintf("https://localhost:%d", core.GetPolicyConfigInstance("testServer").HttpRouterConf().BindPort)

	time.Sleep(200 * time.Millisecond)

	transCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	client := http.Client{Timeout: 2 * time.Second, Transport: transCfg}

	jRadiusRequest := `
	{
		"destination": "{{ .DESTINATION_GROUP }}",
		"packet": {
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
		},
		"perRequestTimeoutSpec": "1s",
		"tries": 1,
		"serverTries": 1
	}
	`

	jRadiusAnswer, err := RouteHttp(client, httpRouterURL+"/routeRadiusRequest?DESTINATION_GROUP=igor-superserver-group", []byte(jRadiusRequest))
	if err != nil {
		t.Fatalf("error routing radius: %s", err)
	}
	radiusAnswer := core.RadiusPacket{}
	if json.Unmarshal(jRadiusAnswer, &radiusAnswer) != nil {
		t.Fatalf("error decoding radius response: %s", err)
	}
	if radiusAnswer.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("radius response does not contain expected radius attribute")
	}

	jRDiameterRequest := `
	{
		"Message": {
			"IsRequest": true,
			"IsProxyable": false,
			"IsError": false,
			"IsRetransmission": false,
			"CommandCode": 2000,
			"ApplicationId": 1000,
			"avps":[
				{"Origin-Host": "server.igorserver"},
				{"Origin-Realm": "igorserver"},
				{"Destination-Realm": "{{ .DESTINATION_REALM }}"},
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
		},
		"TimeoutSpec": "2s"
	}

	`
	jDiameterAnswer, err := RouteHttp(client, httpRouterURL+"/routeDiameterRequest?DESTINATION_REALM=igorsuperserver", []byte(jRDiameterRequest))
	if err != nil {
		t.Fatalf("error routing diameter: %s", err)
	}
	diameterAnswer := core.DiameterMessage{}
	if json.Unmarshal(jDiameterAnswer, &diameterAnswer) != nil {
		t.Fatalf("error decoding diameter response: %s", err)
	}
	if diameterAnswer.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("diameter response does not contain expected diameter attribute")
	}

	val, err := core.GetMetricWithLabels("http_router_exchanges", `{.*path="/routeDiameterRequest".*}`)
	if err != nil {
		t.Fatalf("error getting http_router_exchanges %s", err)
	}
	if val != "1" {
		t.Fatalf("number of http_router_exchanges messages was not 1")
	}
	val, err = core.GetMetricWithLabels("http_router_exchanges", `{.*path="/routeRadiusRequest".*}`)
	if err != nil {
		t.Fatalf("error getting http_router_exchanges %s", err)
	}
	if val != "1" {
		t.Fatalf("number of http_router_exchanges messages was not 1")
	}

	rrouter.Close()
	drouter.Close()
	rsserver.Close()
	dsserver.Close()

	httpRouter.Close()
}
