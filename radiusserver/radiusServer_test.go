package radiusserver

import (
	"context"
	"fmt"
	"igor/config"
	"igor/radiuscodec"
	"igor/router"
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

	// Emulated router channel
	routerChan := make(chan router.RoutableRadiusRequest)

	// Get the configuration
	pci := config.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	ctx, terminateServerSocket := context.WithCancel(context.Background())
	NewRadiusServer(ctx, config.GetPolicyConfigInstance("testServer"), serverConf.BindAddress, serverConf.AuthPort, routerChan)

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

	// Process the request
	rreq := <-routerChan
	t.Log(fmt.Println(rreq.RadiusPacket))
	// Generate answer
	resPacket := radiuscodec.NewRadiusResponse(rreq.RadiusPacket, true)
	resPacket.Add("Class", "this is the response")
	rreq.RChan <- resPacket

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
	if receivedPacket.GetStringAVP("Class") != "this is the response" {
		t.Errorf("unexpected class attribute in response <%s>", receivedPacket.GetStringAVP("Class"))
	}

	terminateServerSocket()

	// Wait fo the socket to be created
	time.Sleep(1000 * time.Millisecond)
}
