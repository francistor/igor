package radiusclient

import (
	"embed"
	"os"
	"testing"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/radiusserver"
)

// Simple handler that generates a success response with the same attributes as in the request
func echoHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	response := core.NewRadiusResponse(request, true)
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
	core.InitPolicyConfigInstance("resources/searchRules.json", "testServer", nil, embed.FS{}, true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func Test1(t *testing.T) {
	// Get the configuration
	pci := core.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := radiusserver.NewRadiusServer(core.GetPolicyConfigInstance("testServer").RadiusClients(), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the server to be created
	time.Sleep(100 * time.Millisecond)

	// Create the RadiusClientSocket
	cchan := make(chan interface{})
	rcs := NewRadiusClientSocket(cchan, "192.168.122.1", 18120)

	// Create a request radius packet
	request := core.NewRadiusRequest(43)
	request.Add("Acct-Session-Id", "session-standard-1")
	request.Add("PSA-Operation", "queryByIP")
	request.Add("Framed-IP-Address", "127.0.1.1")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)
	m1 := ClientRadiusRequestMsg{
		endpoint: "192.168.122.53:1812",
		packet:   request,
		timeout:  1 * time.Second,
		secret:   "secret",
		rchan:    rchan1,
	}
	rcs.SendRadiusRequest(m1)

	// Verify answer
	<-rchan1

	rcs.SetDown()
	rcs.Close()
	rs.Close()

}

func TestRadiusClientSocket(t *testing.T) {
	// Get the configuration
	pci := core.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := radiusserver.NewRadiusServer(core.GetPolicyConfigInstance("testServer").RadiusClients(), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the server to be created
	time.Sleep(100 * time.Millisecond)

	// Create the RadiusClientSocket
	cchan := make(chan interface{})
	rcs := NewRadiusClientSocket(cchan, "127.0.0.1", 18120)

	// Create a request radius packet
	request := core.NewRadiusRequest(1)
	request.Add("User-Name", "myUserName")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)
	m1 := ClientRadiusRequestMsg{
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
	case *core.RadiusPacket:
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
	m2 := ClientRadiusRequestMsg{
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
	case *core.RadiusPacket:
		t.Fatalf("did not get a timeout")
	default:
		t.Fatalf("got %v", v)
	}

	// Terminate the clientsocket
	rcs.SetDown()

	// Wait to receive Socket down
	ev := <-cchan
	switch e := ev.(type) {
	case SocketDownEvent:
		if e.Error != nil {
			t.Fatalf("socketdown event with non nil error %#v", e)
		}
	default:
		t.Fatalf("unexpected event %#v", e)
	}

	// Terminate the client socket
	rcs.Close()

	// Terminate the server
	rs.Close()
}

func TestRadiusClientSocketClose(t *testing.T) {
	// Get the configuration
	core.GetPolicyConfigInstance("testServer")

	// Create the RadiusClientSocket
	cchan := make(chan interface{})
	rcs := NewRadiusClientSocket(cchan, "127.0.0.1", 18120)

	rcs.closeSocket()

	ev := <-cchan
	switch e := ev.(type) {
	case SocketDownEvent:
		if e.Error == nil {
			t.Fatalf("socketdown event with nil error")
		}
	default:
		t.Fatalf("unexpected event %#v", e)
	}

	rcs.Close()
}

func TestRadiusClientOnly(t *testing.T) {
	// Get the configuration
	pci := core.GetPolicyConfigInstance("testServer")
	serverConf := pci.RadiusServerConf()

	// Instantiate a radius server
	rs := radiusserver.NewRadiusServer(core.GetPolicyConfigInstance("testServer").RadiusClients(), serverConf.BindAddress, serverConf.AuthPort, echoHandler)

	// Wait fo the server to be created
	time.Sleep(100 * time.Millisecond)

	// Create the radius client
	rc := NewRadiusClient()

	// Create a request radius packet
	request := core.NewRadiusRequest(1)
	request.Add("User-Name", "myUserName")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)

	rc.RadiusExchange("127.0.0.1:1812", 2000, request, 100*time.Millisecond, 1, "secret", rchan1)

	// Verify answer
	response1 := <-rchan1
	switch v := response1.(type) {
	case error:
		t.Fatalf("received error response: %s", v)
	case *core.RadiusPacket:
		if v.GetStringAVP("User-Name") != "myUserName" {
			t.Fatal("User-Name attribute not found in response")
		}
	default:
		t.Fatalf("got %v", v)
	}

	// Send request to non existing server and get the timeout
	// Create channel for the request
	rchan2 := make(chan interface{}, 1)

	rc.RadiusExchange("127.0.0.1:1888", 18120, request, 100*time.Millisecond, 1, "secret", rchan2)
	response2 := <-rchan2
	switch v := response2.(type) {
	case error:
	case *core.RadiusPacket:
		t.Fatalf("did not get a timeout")
	default:
		t.Fatalf("got %v", v)
	}

	// The following requests will be cancelled, not timed out
	rchan3 := make(chan interface{}, 1)
	rchan4 := make(chan interface{}, 1)
	rc.RadiusExchange("127.0.0.1:1888", 18130, request, 1000*time.Second, 1, "secret", rchan3)
	rc.RadiusExchange("127.0.0.1:1888", 18140, request, 1000*time.Second, 1, "secret", rchan4)

	rc.SetDown()
	<-rchan3
	response4 := <-rchan4
	switch v := response4.(type) {
	case error:
	case *core.RadiusPacket:
		t.Fatalf("did not get a cancellation")
	default:
		t.Fatalf("got %v", v)
	}

	rs.Close()
}

func TestIdentifiers(t *testing.T) {

	// Create the radius client
	rc := NewRadiusClient()

	// Create a request radius packet
	request1 := core.NewRadiusRequest(1)
	request1.Add("User-Name", "myUserName")
	request2 := core.NewRadiusRequest(1)
	request2.Add("User-Name", "myUserName")
	request3 := core.NewRadiusRequest(1)
	request3.Add("User-Name", "myUserName")
	request4 := core.NewRadiusRequest(1)
	request4.Add("User-Name", "myUserName")

	// Create channel for the request
	rchan1 := make(chan interface{}, 1)
	rchan2 := make(chan interface{}, 1)
	rchan3 := make(chan interface{}, 1)
	rchan4 := make(chan interface{}, 1)

	rc.RadiusExchange("1.1.1.1:1812", 2001, request1, 200*time.Millisecond, 2, "secret", rchan1)
	<-rchan1
	rc.RadiusExchange("1.1.1.1:1813", 2001, request2, 200*time.Millisecond, 2, "secret", rchan2)
	<-rchan2
	rc.RadiusExchange("1.1.1.1:1812", 2001, request3, 200*time.Millisecond, 2, "secret", rchan3)
	<-rchan3
	rc.RadiusExchange("1.1.1.1:1813", 2001, request4, 200*time.Millisecond, 2, "secret", rchan4)
	<-rchan4

	rc.SetDown()
	rc.Close()
}
