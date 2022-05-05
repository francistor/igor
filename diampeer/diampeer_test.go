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
	answer := diamcodec.NewDefaultDiameterAnswer(request)
	answer.Add("User-Name", "TestUserNameEcho")

	// Simulate the answer takes some time
	time.Sleep(300 * time.Millisecond)
	return &answer, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitConfigurationInstance("resources/searchRules.json", "unitTestInstance")
	config.InitConfigurationInstance("resources/searchRules.json", "unitTestInstanceOtherClient")

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterPeer(t *testing.T) {

	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	// Needs to have a server configuration as server.igor and one peer also as server.igor (but passive)

	activePeerConfig := config.DiameterPeer{
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
		passivePeer = NewPassiveDiameterPeer("unitTestInstance", routerInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("unitTestInstance", routerInputChannel, activePeerConfig, MyMessageHandler)
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
	request, _ := diamcodec.NewDefaultDiameterRequest("TestApplication", "TestRequest")
	request.Add("User-Name", "TestUserNameRequest")
	response, error := activePeer.DiameterRequest(&request, 2*time.Second)

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
	_, eTimeout := activePeer.DiameterRequest(&request, 10*time.Millisecond)

	if eTimeout == nil {
		t.Fatal("should have got an error")
	} else if eTimeout.Error() != "Timeout" {
		t.Fatal("should have got a timeout")
	}

	// Disonnect peers
	passivePeer.Disengage()
	activePeer.Disengage()

	downEvent1 := <-routerInputChannel
	if _, ok := downEvent1.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
	downEvent2 := <-routerInputChannel
	if _, ok := downEvent2.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}

	// Received PeerDown, we can close
	passivePeer.Close()
	activePeer.Close()
}

func TestDiameterPeerBadActiveClient(t *testing.T) {
	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	// Needs to have a server configuration as server.igor and one peer also as server.igor (but passive)
	// The passive peer will receive a connection from server.igor that will succeed
	// The active peer will establish a connection with BAD.igor but the CEA will report server.igor

	activePeerConfig := config.DiameterPeer{
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
		passivePeer = NewPassiveDiameterPeer("unitTestInstance", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("unitTestInstance", activeInputChannel, activePeerConfig, MyMessageHandler)

	upMsg := <-passiveInputChannel
	// First receives a peerup
	if _, ok := upMsg.(PeerUpEvent); !ok {
		t.Fatal("received initial non PeerUpEvent in passive peer")
	}
	// Then a PeerDownEvent, when the client disconnects
	nextMsg := <-passiveInputChannel
	if _, ok := nextMsg.(PeerDownEvent); !ok {
		t.Fatal("received subsequent non PeerUpDownEvent in passive peer")
	}

	downMsg := <-activeInputChannel
	if _, ok := downMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent")
	}

	// PeerDown received for both
	passivePeer.Close()
	activePeer.Close()
}

func TestDiameterPeerBadPassiveServer(t *testing.T) {
	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	// The active client reports itself as otherclient.igor, which is not recongized by the server
	// The passive peer reports an error (otherclient.igor not known),

	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igor",
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
		passivePeer = NewPassiveDiameterPeer("unitTestInstance", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("unitTestInstanceOtherClient", activeInputChannel, activePeerConfig, MyMessageHandler)

	upMsg := <-passiveInputChannel
	if _, ok := upMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}
	// Received PeerDown event, can be closed

	downMsg := <-activeInputChannel
	if _, ok := downMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	// Close peers
	activePeer.Close()
	passivePeer.Close()
}
