package diampeer

import (
	"igor/config"
	"igor/diamcodec"
	"net"
	"os"
	"testing"
	"time"
)

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	answer.Add("User-Name", "TestUserNameEcho")

	// Simulate the answer takes some time
	time.Sleep(300 * time.Millisecond)
	return &answer, nil
}

func TestMain(m *testing.M) {

	// Initialize logging
	config.SetupLogger()

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "unitTestInstance"
	config.Config.Init(bootstrapFile, instanceName)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterPeer(t *testing.T) {

	var aPeer *DiameterPeer
	var bPeer *DiameterPeer

	// Needs to have a server configuration as server.igor and one peer also as server.igor (but passive)

	bPeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igor",
		IPAddress:               "127.0.0.1",
		Port:                    3868,
		ConnectionPolicy:        "active",
		OriginNetwork:           "127.0.0.0/8",
		WatchdogIntervalMillis:  30000,
		ConnectionTimeoutMillis: 3000,
	}

	var routerInputChannel = make(chan interface{}, 100)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		aPeer = NewPassiveDiameterPeer(routerInputChannel, conn, MyMessageHandler)
	}()

	bPeer = NewActiveDiameterPeer(routerInputChannel, bPeerConfig, MyMessageHandler)
	connectedMsg1 := <-routerInputChannel
	if p1, ok := connectedMsg1.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent")
	} else if p1.DiameterHost != "server.igor" {
		t.Fatalf("received %s as Origin-Host", p1.DiameterHost)
	}
	connectedMsg2 := <-routerInputChannel
	if p2, ok := connectedMsg2.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent")
	} else if p2.DiameterHost != "server.igor" {
		t.Fatalf("received %s as Origin-Host", p2.DiameterHost)
	}

	// Correct response
	request, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	request.Add("User-Name", "TestUserNameRequest")
	response, error := bPeer.DiameterRequest(&request, 2*time.Second)

	if error != nil {
		t.Fatal("bad response", err)
	}
	userNameAVP, error := response.GetAVP("User-Name")
	if error != nil {
		t.Fatal("bad AVP", err)
	}
	if userNameAVP.GetString() != "TestUserNameEcho" {
		t.Fatal("bad AVP content", userNameAVP.GetString())
	}

	// Simulate a timeout. The handler takes more time than this
	_, eTimeout := bPeer.DiameterRequest(&request, 10*time.Millisecond)

	if eTimeout == nil {
		t.Fatal("should have got an error")
	} else if eTimeout.Error() != "Timeout" {
		t.Fatal("should have got a timeout")
	}

	aPeer.Disengage()
	bPeer.Disengage()
	downEvent1 := <-routerInputChannel
	if _, ok := downEvent1.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
	downEvent2 := <-routerInputChannel
	if _, ok := downEvent2.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}

	aPeer.Close()
	bPeer.Close()
}

func TestDiameterPeerBadClient(t *testing.T) {
	var aPeer *DiameterPeer
	var bPeer *DiameterPeer

	// Needs to have a server configuration as server.igor and one peer also as server.igor (but passive)

	bPeerConfig := config.DiameterPeer{
		DiameterHost:            "BAD.igor",
		IPAddress:               "127.0.0.1",
		Port:                    3868,
		ConnectionPolicy:        "active",
		OriginNetwork:           "127.0.0.0/8",
		WatchdogIntervalMillis:  30000,
		ConnectionTimeoutMillis: 3000,
	}

	var passiveInputChannel = make(chan interface{}, 100)
	var activeInputChannel = make(chan interface{}, 100)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		aPeer = NewPassiveDiameterPeer(passiveInputChannel, conn, MyMessageHandler)
	}()

	bPeer = NewActiveDiameterPeer(activeInputChannel, bPeerConfig, MyMessageHandler)

	upMsg := <-passiveInputChannel
	if _, ok := upMsg.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent in passive peer")
	}
	downMsg := <-activeInputChannel
	if _, ok := downMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent")
	}

	// Close peers
	aPeer.Close()
	bPeer.Close()
}
