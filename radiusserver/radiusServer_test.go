package radiusserver

import (
	"context"
	"igor/config"
	"igor/radiuscodec"
	"net"
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

func TestRadiusServer(t *testing.T) {

	// Get the configuration
	pci := config.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	ctx, terminateServerSocket := context.WithCancel(context.Background())
	NewRadiusServer(ctx, config.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the socket to be created
	time.Sleep(100 * time.Millisecond)

	// Create a request radius packet
	request := radiuscodec.NewRadiusRequest(1)
	request.Add("User-Name", "myUserName")

	// Send a request using a local socket
	clientSocket, err := net.ListenPacket("udp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	requestBytes, err := request.ToBytes("secret", 100)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1812")
	if err != nil {
		t.Fatal(err)
	}
	clientSocket.WriteTo(requestBytes, addr)

	// Get response
	responseBuffer := make([]byte, 4096)
	_, _, err = clientSocket.ReadFrom(responseBuffer)
	if err != nil {
		t.Fatal(err)
	}
	receivedPacket, err := radiuscodec.RadiusPacketFromBytes(responseBuffer, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if receivedPacket.GetStringAVP("User-Name") != "myUserName" {
		t.Errorf("unexpected class attribute in response <%s>", receivedPacket.GetStringAVP("User-Name"))
	}

	terminateServerSocket()

	// Wait fo the socket to be created
	time.Sleep(1000 * time.Millisecond)
}

// Simple handler that generates a success response with the same attributes as in the request
func echoHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {

	response := radiuscodec.NewRadiusResponse(request, true)
	for i := range request.AVPs {
		response.AddAVP(&request.AVPs[i])
	}

	return response, nil

}
