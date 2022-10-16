package diampeer

// TODO: connection cannot be established with peer. DWA not neceived

import (
	"igor/config"
	"igor/diamcodec"
	"igor/instrumentation"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// This message handler parses the Igor-Command, which may specify
// whether to introduce a small delay (value "Slow") or a big one (value "VerySlow")
// A Class attribute with the value "TestUserNameEcho" is added to the answer
func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {

	answer := diamcodec.NewDiameterAnswer(request).
		AddOriginAVPs(config.GetPolicyConfig()).
		Add("Class", "TestUserNameEcho")

	command := request.GetStringAVP("Igor-Command")
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

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitPolicyConfigInstance("resources/searchRules.json", "testServer", true)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownServer", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testServerBadOriginNetwork", false)

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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections and spawning DiameterPeer objects.
	// This emulates the code to be found in the DiameterRouter
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	// The configuration of the Peer is set above, with a small watchdog interval
	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// PeerUp event sent by the passive DiameterPeer
	// Check that it is a PeerUpEvent and that the reported Origin-Host is client.igorclient
	passiveUp := <-passiveControlChannel
	if pu, ok := passiveUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for passive peer")
	} else if pu.DiameterHost != "client.igorclient" {
		t.Fatalf("received %s as Origin-Host", pu.DiameterHost)
	}

	// PeerUp event sent by the active DiameterPeer
	// Check that it is a PeerUpEvent and that the reported Origin-Host is server.igorserver
	activeUp := <-activeControlChannel
	if au, ok := activeUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for active peer")
	} else if au.DiameterHost != "server.igorserver" {
		t.Fatalf("received %s as Origin-Host", au.DiameterHost)
	}

	// Wait a while to have some DWR exchanged
	time.Sleep(1 * time.Second)

	// Correct response
	request, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	request.AddOriginAVPs(config.GetPolicyConfigInstance("testClient"))
	request.Add("User-Name", "TestUserNameRequest")
	request.Add("Destination-Realm", "igorserver")
	var rc1 = make(chan interface{}, 1)
	activePeer.DiameterExchange(request, 2*time.Second, rc1)

	a1 := <-rc1
	switch v := a1.(type) {
	case error:
		t.Fatal("bad response", err)
	case *diamcodec.DiameterMessage:
		classAVP, error := v.GetAVP("Class")
		if error != nil {
			t.Fatal("bad AVP", err)
		}
		if classAVP.GetString() != "TestUserNameEcho" {
			t.Fatal("bad AVP content", classAVP.GetString())
		}
	}

	// Simulate a timeout. The handler takes more time than this
	request.Add("Igor-Command", "Slow")
	var rc2 = make(chan interface{}, 1)
	activePeer.DiameterExchange(request, 50*time.Millisecond, rc2)

	a2 := <-rc2
	switch v := a2.(type) {
	case error:
	default:
		t.Fatalf("should have got a timeout but got %v", v)
	}

	// Check metrics. Getting the metrics aggregating by application and command
	metrics := instrumentation.MS.DiameterQuery("DiameterRequestsReceived", nil, []string{"AP", "CM"})
	// Should have received two TestApplication / TestRequest messages
	if metric, ok := metrics[instrumentation.PeerDiameterMetricKey{AP: "TestApplication", CM: "TestRequest"}]; !ok {
		t.Fatal("bad metrics for TestApplication and TestRequest")
	} else {
		if metric != 2 {
			t.Fatalf("bad metrics value for TestApplication and TestRequest: %d", metric)
		}
	}
	// Should have received several Base / Device-Watchdog
	if metric, ok := metrics[instrumentation.PeerDiameterMetricKey{AP: "Base", CM: "Device-Watchdog"}]; !ok {
		t.Fatal("bad metrics for Base and Device-Watchdog")
	} else {
		if metric < 2 {
			t.Fatalf("bad metrics value for Base and Device-Watchdog: %d", metric)
		}
	}

	// Aggregate timeouts per Peer. Getting the metrics aggregated by Peer
	metrics = instrumentation.MS.DiameterQuery("DiameterRequestsTimeout", nil, []string{"Peer"})
	if metric, ok := metrics[instrumentation.PeerDiameterMetricKey{Peer: "server.igorserver"}]; !ok {
		t.Fatal("bad timeout metrics")
	} else {
		if metric != 1 {
			t.Fatalf("bad timeouts metrics value: %d", metric)
		}
	}

	// t.Log(metrics)

	// Disonnect peers
	passivePeer.SetDown()
	activePeer.SetDown()

	// Wait for PeerDown events
	downEvent1 := <-passiveControlChannel
	if _, ok := downEvent1.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
	downEvent2 := <-activeControlChannel
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
	// The active peer will establish a connection with unkserver.igorserver but the CEA will report server.igor
	activePeerConfig := config.DiameterPeer{
		DiameterHost:            "unkserver.igorserver",
		IPAddress:               "127.0.0.1",
		Port:                    3868,
		ConnectionPolicy:        "active",
		OriginNetwork:           "127.0.0.0/8",
		WatchdogIntervalMillis:  30000,
		ConnectionTimeoutMillis: 3000,
	}

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClientUnknownServer", activeControlChannel, activePeerConfig, MyMessageHandler)

	// First receives a PeerUp. the passive sees a connection from client.igorclient which is correct
	upMsg := <-passiveControlChannel
	if _, ok := upMsg.(PeerUpEvent); !ok {
		t.Fatal("received initial non PeerUpEvent in passive peer")
	}
	// Then a PeerDownEvent, when the client disconnects because the server reported a name different than unkserver.igorserver
	nextMsg := <-passiveControlChannel
	if _, ok := nextMsg.(PeerDownEvent); !ok {
		t.Fatal("received subsequent non PeerUpDownEvent in passive peer")
	}

	// The active peer gets an error because the origin host reported is not unkserver.igor
	downMsg := <-activeControlChannel
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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClientUnknownClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// The server receives an unknown origin-host from the client
	passiveMsg := <-passiveControlChannel
	if _, ok := passiveMsg.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}
	// Received PeerDown event, can be closed
	activeMsg := <-activeControlChannel
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

	var activeControlChannel = make(chan interface{}, 16)

	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	downMsg := <-activeControlChannel
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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		// The server expects connections from client.igorclient in the 1.0.0.0/8 network
		passivePeer = NewPassiveDiameterPeer("testServerBadOriginNetwork", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// Both peers get peer down event
	msg1 := <-activeControlChannel
	if _, ok := msg1.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	msg2 := <-passiveControlChannel
	if _, ok := msg2.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}

	// Received PeerDown, we can close
	passivePeer.Close()
	activePeer.Close()
}

func TestRequestsCancellation(t *testing.T) {
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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// Get both PeerUp events
	passiveUp := <-passiveControlChannel
	if pu, ok := passiveUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for passive peer")
	} else if pu.DiameterHost != "client.igorclient" {
		t.Fatalf("received %s as Origin-Host", pu.DiameterHost)
	}
	activeUp := <-activeControlChannel
	if au, ok := activeUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for active peer")
	} else if au.DiameterHost != "server.igorserver" {
		t.Fatalf("received %s as Origin-Host", au.DiameterHost)
	}

	// Simulate two long requests
	request1, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	request1.AddOriginAVPs(config.GetPolicyConfigInstance("testClient"))
	request1.Add("Igor-Command", "Slow")
	request2, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	request2.AddOriginAVPs(config.GetPolicyConfigInstance("testClient"))
	request2.Add("Igor-Command", "Slow")

	rc1 := make(chan interface{}, 1)
	rc2 := make(chan interface{}, 1)
	activePeer.DiameterExchange(request1, 300*time.Second, rc1)
	activePeer.DiameterExchange(request2, 300*time.Second, rc2)

	// Disengage Peer
	activePeer.SetDown()
	// Wait for Peer down
	<-activeControlChannel

	// Check cancellation of both messages
	resp1 := <-rc1
	r, ok := resp1.(error)
	if !ok {
		t.Fatal("did not get an error message")
	} else if !strings.Contains(r.Error(), "cancelled") {
		t.Fatalf("wrong error message %s", r.Error())
	}
	resp2 := <-rc2
	r, ok = resp2.(error)
	if !ok {
		t.Fatal("did not get an error message")
	} else if !strings.Contains(r.Error(), "cancelled") {
		t.Fatalf("wrong error message %s", r.Error())
	}

	passivePeer.SetDown()
	<-passiveControlChannel

	// Close
	activePeer.Close()
	passivePeer.Close()
}

func TestSocketError(t *testing.T) {
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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// Get both PeerUp events
	passiveUp := <-passiveControlChannel
	if pu, ok := passiveUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for passive peer")
	} else if pu.DiameterHost != "client.igorclient" {
		t.Fatalf("received %s as Origin-Host", pu.DiameterHost)
	}
	activeUp := <-activeControlChannel
	if au, ok := activeUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for active peer")
	} else if au.DiameterHost != "server.igorserver" {
		t.Fatalf("received %s as Origin-Host", au.DiameterHost)
	}

	// Force error in client
	activePeer.tstForceSocketError()

	// Both peers get peer down event
	msg1 := <-activeControlChannel
	if _, ok := msg1.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	msg2 := <-passiveControlChannel
	if _, ok := msg2.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}

	// Close
	activePeer.Close()
	passivePeer.Close()
}

func TestDisconnectMessage(t *testing.T) {
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

	var passiveControlChannel = make(chan interface{}, 16)
	var activeControlChannel = make(chan interface{}, 16)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		passivePeer = NewPassiveDiameterPeer("testServer", passiveControlChannel, conn, MyMessageHandler)
	}()

	activePeer = NewActiveDiameterPeer("testClient", activeControlChannel, activePeerConfig, MyMessageHandler)

	// Get both PeerUp events
	passiveUp := <-passiveControlChannel
	if pu, ok := passiveUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for passive peer")
	} else if pu.DiameterHost != "client.igorclient" {
		t.Fatalf("received %s as Origin-Host", pu.DiameterHost)
	}
	activeUp := <-activeControlChannel
	if au, ok := activeUp.(PeerUpEvent); !ok {
		t.Fatal("received non PeerUpEvent for active peer")
	} else if au.DiameterHost != "server.igorserver" {
		t.Fatalf("received %s as Origin-Host", au.DiameterHost)
	}

	// Force disconnect peer
	activePeer.tstSendDisconnectPeer()

	// Both peers get peer down event
	msg1 := <-activeControlChannel
	if _, ok := msg1.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in active peer")
	}

	msg2 := <-passiveControlChannel
	if _, ok := msg2.(PeerDownEvent); !ok {
		t.Fatal("received non PeerDownEvent in passive peer")
	}

	// Close
	activePeer.Close()
	passivePeer.Close()
}
