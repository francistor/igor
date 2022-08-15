package httprouter

import (
	"crypto/tls"
	"encoding/json"
	"igor/config"
	"igor/diamcodec"
	"igor/radiuscodec"
	"igor/router"
	"net/http"
	"os"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// This message handler parses the franciscocardosogil1-Command, which may specify
// whether to introduce a small delay (value "Slow") or a big one (value "VerySlow")
// A User-Name attribute with the value "TestUserNameEcho" is added to the answer
func diameterHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	answer.Add("User-Name", "EchoLocal")
	answer.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)

	command := request.GetStringAVP("franciscocardosogil-Command")
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
func radiusHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	resp := radiuscodec.NewRadiusResponse(request, true)
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
	config.InitPolicyConfigInstance(bootstrapFile, "testServer", true)
	config.InitPolicyConfigInstance(bootstrapFile, "testSuperServer", false)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestHttpRouterHandler(t *testing.T) {

	rrouter := router.NewRadiusRouter("testServer", nil)
	drouter := router.NewDiameterRouter("testServer", nil)
	rsserver := router.NewRadiusRouter("testSuperServer", radiusHandler)
	dsserver := router.NewDiameterRouter("testSuperServer", diameterHandler)

	httpRouter := NewHttpRouter("testServer", drouter, rrouter)

	time.Sleep(200 * time.Millisecond)

	httpRouter.Close()

	transCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	client := http.Client{Timeout: 2 * time.Second, Transport: transCfg}

	jRadiusRequest := `
	{
		"destination": "igor-superserver-group",
		"packet": {
			"Code": 1,
			"AVPs":[
				{"Igor-OctetsAttribute": "0102030405060708090a0b"},
				{"Igor-StringAttribute": "stringvalue"},
				{"Igor-IntegerAttribute": "Zero"},
				{"Igor-IntegerAttribute": "1"},
				{"Igor-IntegerAttribute": 1},
				{"Igor-AddressAttribute": "127.0.0.1:1"},
				{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
				{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
				{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
				{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
				{"Igor-Integer64Attribute": 999999999999},
				{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
				{"User-Name":"MyUserName"}
			]
		},
		"perRequestTimeoutSpec": "1s",
		"tries": 1,
		"serverTries": 1
	}
	`

	jRadiusAnswer, err := RouteRadius(rrouter, client, "/RouteRadiusRequest", []byte(jRadiusRequest))
	if err != nil {
		t.Fatalf("error routing radius: %s", err)
	}
	radiusAnswer := radiuscodec.RadiusPacket{}
	if json.Unmarshal(jRadiusAnswer, &radiusAnswer) != nil {
		t.Fatalf("error decoding radius response: %s", err)
	}
	if radiusAnswer.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("radius response does not contain expected radius attribute")
	}

	jDiameterRequest := `
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
				{"Destination-Realm": "igorsuperserver"},
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
		},
		"TimeoutSpec": "2s"
	}

	`
	jDiameterAnswer, err := RouteDiameter(drouter, client, "/RouteDiameterRequest", []byte(jDiameterRequest))
	if err != nil {
		t.Fatalf("error routing radius: %s", err)
	}
	diameterAnswer := diamcodec.DiameterMessage{}
	if json.Unmarshal(jDiameterAnswer, &diameterAnswer) != nil {
		t.Fatalf("error decoding diameter response: %s", err)
	}
	if diameterAnswer.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("radius response does not contain expected diameter attribute")
	}

	// TODO: Check stats

	rrouter.SetDown()
	drouter.SetDown()
	rsserver.SetDown()
	dsserver.SetDown()

	rrouter.Close()
	drouter.Close()
	rsserver.Close()
	dsserver.Close()

	httpRouter.Close()
}
