package radiusclient

import (
	"igor/config"
	"igor/radiuscodec"
	"igor/radiusserver"
	"os"
	"testing"
	"time"
)

// Simple handler that generates a success response with the same attributes as in the request
func echoHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {

	response := radiuscodec.NewRadiusResponse(request, true)
	for i := range request.AVPs {
		response.AddAVP(&request.AVPs[i])
	}

	timeout := request.GetIntAVP("Session-Timeout")
	if timeout != 0 {
		time.Sleep(time.Duration(timeout) * time.Second)
	}

	return response, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitPolicyConfigInstance("resources/searchRules.json", "testServer", true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestRadiusClientSocket(t *testing.T) {
	// Get the configuration
	pci := config.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := radiusserver.NewRadiusServer(config.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the server to be created
	time.Sleep(100 * time.Millisecond)

	// Create the RadiusClientSocket
	cchan := make(chan interface{})
	rcs := NewRadiusClientSocket(cchan, pci, "127.0.0.1", 18120)

	// Create a request radius packet
	request := radiuscodec.NewRadiusRequest(1)
	request.Add("User-Name", "myUserName")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)
	m1 := RadiusRequestMsg{
		endpoint: "127.0.0.1:1812",
		packet:   request,
		timeout:  1 * time.Second,
		secret:   "secret",
		rchan:    rchan1,
	}
	rcs.SendRadiusRequest(m1)

	// Verify answer
	response := <-rchan1
	switch v := response.(type) {
	case error:
		t.Fatalf("received error response: %s", v)
	case *radiuscodec.RadiusPacket:
		if v.GetStringAVP("User-Name") != "myUserName" {
			t.Fatal("User-Name attribute not found in response")
		}
	default:
		t.Fatalf("got %v", v)
	}

	// Force a timeout
	request.Add("Session-Timeout", 1)
	// Create channel for the request
	rchan2 := make(chan interface{}, 1)
	m2 := RadiusRequestMsg{
		endpoint: "127.0.0.1:1812",
		packet:   request,
		timeout:  500 * time.Millisecond,
		secret:   "secret",
		rchan:    rchan2,
	}

	rcs.SendRadiusRequest(m2)

	// Verify answer
	response = <-rchan2
	switch v := response.(type) {
	case error:
	case *radiuscodec.RadiusPacket:
		t.Fatalf("did not get a timeout")
	default:
		t.Fatalf("got %v", v)
	}

	// Terminate the clientsocket
	rcs.SetDown()

	// Wait to receive Socket down
	<-cchan

	rcs.Close()

	// Terminate the server
	rs.Close()
}

func TestRadiusClientOnly(t *testing.T) {
	// Get the configuration
	pci := config.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := radiusserver.NewRadiusServer(config.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the server to be created
	time.Sleep(100 * time.Millisecond)

	// Create the radius client
	rc := NewRadiusClient(pci)

	// Create a request radius packet
	request := radiuscodec.NewRadiusRequest(1)
	request.Add("User-Name", "myUserName")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)

	rc.RadiusExchange("servername", "127.0.0.1:1812", 2000, request, 100*time.Millisecond, "secret", rchan1)

	// Verify answer
	response1 := <-rchan1
	switch v := response1.(type) {
	case error:
		t.Fatalf("received error response: %s", v)
	case *radiuscodec.RadiusPacket:
		if v.GetStringAVP("User-Name") != "myUserName" {
			t.Fatal("User-Name attribute not found in response")
		}
	default:
		t.Fatalf("got %v", v)
	}

	// Send request to non existing server and get the timeout
	// Create channel for the request
	rchan2 := make(chan interface{}, 1)

	rc.RadiusExchange("servername", "127.0.0.1:1888", 18120, request, 100*time.Millisecond, "secret", rchan2)
	response2 := <-rchan2
	switch v := response2.(type) {
	case error:
	case *radiuscodec.RadiusPacket:
		t.Fatalf("did not get a timeout")
	default:
		t.Fatalf("got %v", v)
	}

	// The following requests will be cancelled, not timed out
	rchan3 := make(chan interface{}, 1)
	rchan4 := make(chan interface{}, 1)
	rc.RadiusExchange("servername", "127.0.0.1:1888", 18130, request, 1000*time.Second, "secret", rchan3)
	rc.RadiusExchange("servername", "127.0.0.1:1888", 18140, request, 1000*time.Second, "secret", rchan4)

	rc.SetDown()
	<-rchan3
	response4 := <-rchan4
	switch v := response4.(type) {
	case error:
	case *radiuscodec.RadiusPacket:
		t.Fatalf("did not get a cancellation")
	default:
		t.Fatalf("got %v", v)
	}

	rs.Close()
}

// TODO: Test cancellation of outstanding requests
