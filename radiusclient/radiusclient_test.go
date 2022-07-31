package radiusClient

import (
	"context"
	"igor/config"
	"igor/radiuscodec"
	"igor/radiusserver"
	"os"
	"testing"
	"time"
)

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
	ctx, terminateServerSocket := context.WithCancel(context.Background())
	radiusserver.NewRadiusServer(ctx, config.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

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
	rcs.RadiusExchange("127.0.0.1:1812", request, 1*time.Second, "secret", rchan1)

	// Verify answer
	response := <-rchan1
	switch v := response.(type) {
	case error:
		t.Fatalf("received error response: %s", v)
	case *radiuscodec.RadiusPacket:
	default:
		t.Fatalf("got %v", v)
	}

	// Force a timeout
	request.Add("Session-Timeout", 1)
	// Create channel for the request
	rchan2 := make(chan interface{}, 1)
	rcs.RadiusExchange("127.0.0.1:1812", request, 500*time.Millisecond, "secret", rchan2)

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
	terminateServerSocket()
}

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
