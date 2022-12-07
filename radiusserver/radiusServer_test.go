package radiusserver

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/francistor/igor/core"
)

// Simple handler that generates a success response with the same attributes as in the request
func echoHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	response := core.NewRadiusResponse(request, true)
	for i := range request.AVPs {
		response.AddAVP(&request.AVPs[i])
	}

	return response, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	core.InitPolicyConfigInstance("resources/searchRules.json", "testServer", true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestRadiusServer(t *testing.T) {

	theUserName := "myUserName"

	// Get the configuration
	pci := core.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := NewRadiusServer(core.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the socket to be created
	time.Sleep(100 * time.Millisecond)

	// Create a request radius packet
	request := core.NewRadiusRequest(core.ACCESS_REQUEST)
	request.Add("User-Name", theUserName)

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
	receivedPacket, err := core.RadiusPacketFromBytes(responseBuffer, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if receivedPacket.GetStringAVP("User-Name") != theUserName {
		t.Errorf("unexpected class attribute in response <%s>", receivedPacket.GetStringAVP("User-Name"))
	}

	rs.Close()
}
