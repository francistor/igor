package diampeer

// TODO: connection cannot be established with peer. DWA not neceived

import (
	"igor/config"
	"igor/diamcodec"
	"igor/instrumentation"
	"net"
	"os"
	"testing"
	"time"
)

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDefaultDiameterAnswer(request)
	answer.Add("User-Name", "TestUserNameEcho")

	command := request.GetStringAVP("franciscocardosogil-Command")
	switch command {
	case "Slow":
		// Simulate the answer takes some time
		time.Sleep(300 * time.Millisecond)
	case "VerySlow":
		// Simulate the answer takes more time
		time.Sleep(5000 * time.Millisecond)
	}

	return &answer, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitConfigurationInstance("resources/searchRules.json", "testServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testServerBadOriginNetwork")

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterPeerOK(t *testing.T) {

	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igorserver",
		IPAddress:               "127.0.0.1",
		Port:                    3868,
		ConnectionPolicy:        "active",
		OriginNetwork:           "127.0.0.0/8",
		WatchdogIntervalMillis:  300, // Small DWR interval!
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
		passivePeer = NewPassiveDiameterPeer("testServer", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeInputChannel, activePeerConfig, MyMessageHandler)
	passiveUp := <-passiveInputChannel
	if pu, ok := passiveUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for passive peer")
	} else if pu.DiameterHost != "client.igorclient" {
		t.Fatalf("received %s as Origin-Host", pu.DiameterHost)
	}
	activeUp := <-activeInputChannel
	if au, ok := activeUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for active peer")
	} else if au.DiameterHost != "server.igorserver" {
		t.Fatalf("received %s as Origin-Host", au.DiameterHost)
	}

	// Wait a while to have some DWR exchanged
	time.Sleep(1 * time.Second)

	// Correct response
	request, _ := diamcodec.NewDefaultDiameterRequest("TestApplication", "TestRequest")
	request.Add("User-Name", "TestUserNameRequest")
	response, error := activePeer.DiameterExhangeWithAnswer(&request, 2*time.Second)

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
	request.Add("franciscocardosogil-Command", "Slow")
	_, eTimeout := activePeer.DiameterExhangeWithAnswer(&request, 10*time.Millisecond)

	if eTimeout == nil {
		t.Fatal("should have got an error")
	} else if eTimeout.Error() != "Timeout" {
		t.Fatal("should have got a timeout")
	}

	// Check metrics
	metrics := instrumentation.MS.DiameterQuery("DiameterRequestsReceived", nil, []string{"AP", "CM"})
	// Should have received two TestApplication / TestRequest messages
	k1 := instrumentation.DiameterMetricKey{AP: "TestApplication", CM: "TestRequest"}
	if metric, ok := metrics[k1]; !ok {
		t.Fatal("bad metrics for TestApplication and TestRequest")
	} else {
		if metric != 2 {
			t.Fatalf("bad metrics value for TestApplication and TestRequest: %d", metric)
		}
	}
	// Should have received several Base / Device-Watchdog
	k2 := instrumentation.DiameterMetricKey{AP: "Base", CM: "Device-Watchdog"}
	if metric, ok := metrics[k2]; !ok {
		t.Fatal("bad metrics for Base and Device-Watchdog")
	} else {
		if metric < 2 {
			t.Fatalf("bad metrics value for Base and Device-Watchdog: %d", metric)
		}
	}

	// Aggregate timeouts per Peer
	metrics = instrumentation.MS.DiameterQuery("DiameterRequestsTimeout", nil, []string{"Peer"})
	k3 := instrumentation.DiameterMetricKey{Peer: "server.igorserver"}
	if metric, ok := metrics[k3]; !ok {
		t.Fatal("bad timeouts metrics")
	} else {
		if metric != 1 {
			t.Fatalf("bad timeouts metrics value: %d", metric)
		}
	}

	// t.Log(metrics)

	// Disonnect peers
	passivePeer.SetDown()
	activePeer.SetDown()

	downEvent1 := <-passiveInputChannel
	if _, ok := downEvent1.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
	downEvent2 := <-activeInputChannel
	if _, ok := downEvent2.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}

	// Received PeerDown, we can close
	passivePeer.Close()
	activePeer.Close()
}

func TestDiameterPeerBadServerName(t *testing.T) {
	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	// The passive peer will receive a connection from client.igor that will succeed
	// The active peer will establish a connection with unkserver.igor but the CEA will report server.igor
	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "unkserver.igorserver",
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
		passivePeer = NewPassiveDiameterPeer("testServer", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClientUnknownServer", activeInputChannel, activePeerConfig, MyMessageHandler)

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

	// The active peer gets an error because the origin host reported is not unkserver.igor
	downMsg := <-activeInputChannel
	if _, ok := downMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent")
	}

	// PeerDown received for both
	passivePeer.Close()
	activePeer.Close()
}

func TestDiameterPeerBadClientName(t *testing.T) {
	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	// The active client reports itself as unkclient.igor, which is not recongized by the server
	// The passive peer reports an error (unkclient.igor not known),

	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igorserver",
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
		passivePeer = NewPassiveDiameterPeer("testServer", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClientUnknownClient", activeInputChannel, activePeerConfig, MyMessageHandler)

	passiveMsg := <-passiveInputChannel
	if _, ok := passiveMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}
	// Received PeerDown event, can be closed
	activeMsg := <-activeInputChannel
	if _, ok := activeMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	// Close peers
	activePeer.Close()
	passivePeer.Close()
}

func TestDiameterPeerUnableToConnect(t *testing.T) {
	var activePeer *DiameterPeer

	// The active client tries to connect to an unavailable server
	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igorserver",
		IPAddress:               "1.0.0.1",
		Port:                    3868,
		ConnectionPolicy:        "active",
		OriginNetwork:           "1.0.0.0/8",
		WatchdogIntervalMillis:  30000,
		ConnectionTimeoutMillis: 2000,
	}

	var activeInputChannel = make(chan interface{}, 100)

	activePeer = NewActiveDiameterPeer("testClient", activeInputChannel, activePeerConfig, MyMessageHandler)

	downMsg := <-activeInputChannel
	if _, ok := downMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	// Close peers
	activePeer.Close()
}

func TestBadOriginNetwork(t *testing.T) {

	var passivePeer *DiameterPeer
	var activePeer *DiameterPeer

	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "server.igorserver",
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
		passivePeer = NewPassiveDiameterPeer("testServerBadOriginNetwork", passiveInputChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeInputChannel, activePeerConfig, MyMessageHandler)

	// Both peers get peer down event
	msg1 := <-activeInputChannel
	if _, ok := msg1.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	msg2 := <-passiveInputChannel
	if _, ok := msg2.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}

	// Received PeerDown, we can close
	passivePeer.Close()
	activePeer.Close()
}
