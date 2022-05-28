package diamserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/instrumentation"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitConfigurationInstance("resources/searchRules.json", "testServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testSuperServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownServer")

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestBasicSetup(t *testing.T) {
	time.Sleep(3 * time.Second)
	superServerPM := NewDiameterPeerManager("testSuperServer")
	time.Sleep(150 * time.Millisecond)
	serverPM := NewDiameterPeerManager("testServer")
	time.Sleep(150 * time.Millisecond)
	clientPM := NewDiameterPeerManager("testClient")

	// Bad peers
	// This sleep time is important. Otherwise another client presenting himself
	// as client.igor generates a race condition and none of the client.igor
	// peers gets engaged
	time.Sleep(200 * time.Millisecond)
	NewDiameterPeerManager("testClientUnknownClient")
	NewDiameterPeerManager("testClientUnknownServer")

	// Time to settle connections
	time.Sleep(1 * time.Second)

	// Uncomment to debug
	/*
		j, _ := json.Marshal(instrumentation.MS.PeersTableQuery())
		fmt.Println(PrettyPrintJSON(j))
	*/

	// Get the current peer status
	peerTables := instrumentation.MS.PeersTableQuery()

	// The testClient PeerManager will have an established connection
	// to server.igor but not one to unreachableserver.igor
	clientTable := peerTables["testClient"]
	clientPeerServer := findPeer("server.igor", clientTable)
	if clientPeerServer.IsEngaged != true {
		t.Error("server.igor not engaged in client peers table")
	}
	clientPeerBadServer := findPeer("unreachableserver.igor", clientTable)
	if clientPeerBadServer.IsEngaged != false {
		t.Error("unreachableserver.igor engaged in client peers table")
	}

	// The testServer PeerManager will have two established connections
	// with client.igor and superserver.igorsuperserver
	serverTable := peerTables["testServer"]
	serverPeerClient := findPeer("client.igor", serverTable)
	if serverPeerClient.IsEngaged != true {
		t.Error("server.igor not engaged in server peers table")

	}
	serverPeerSuperServer := findPeer("superserver.igorsuperserver", serverTable)
	if serverPeerSuperServer.IsEngaged != true {
		t.Error("badserver.igor engaged in server peers table")
	}

	// The testSuperServer will have an established connection with
	// server.igor
	superserverTable := peerTables["testSuperServer"]
	superserverPeerServer := findPeer("server.igor", superserverTable)
	if superserverPeerServer.IsEngaged != true {
		t.Error("server.igor not engaged in superserverserver peers table")
	}

	// Bad clients
	// testClientUnknownClient tries to register with server.igor with
	// a Diameter-Host name that is not recognized by the server
	unkClientTable := peerTables["testClientUnknownClient"]
	clientPeerUnknownClient := findPeer("server.igor", unkClientTable)
	if clientPeerUnknownClient.IsEngaged != false {
		t.Error("server.igor engaged in unknownclient peers table")
	}

	// testClientUnknownServer tries to register with a server in the
	// same address where server.igor is lisening but expecting another
	// server name
	unkServerClientTable := peerTables["testClientUnknownServer"]
	clientPeerUnknownServer := findPeer("server.igor", unkServerClientTable)
	if clientPeerUnknownServer.IsEngaged != false {
		t.Error("server.igor engaged in unknownserver peers table")
	}

	// Close PeerManagers
	serverPM.Close()
	<-serverPM.ManagerDoneChannel
	t.Log("Server PeerManager terminated")

	superServerPM.Close()
	<-superServerPM.ManagerDoneChannel
	t.Log("SuperServer PeerManager terminated")

	clientPM.Close()
	<-clientPM.ManagerDoneChannel
	t.Log("Client PeerManager terminated")

}

// Client will send message to Server, which will handle locally
// The two types of routes are tested here
func TestRouteMessage(t *testing.T) {

	NewDiameterPeerManager("testServer")
	time.Sleep(150 * time.Millisecond)
	client := NewDiameterPeerManager("testClient")

	// Some time to settle
	time.Sleep(200 * time.Millisecond)

	// Build request
	request, _ := diamcodec.NewDefaultDiameterRequest("TestApplication", "TestRequest")
	request.Add("Destination-Realm", "igorsuperserver")
	request.Add("User-Name", "TestUserNameRequest")
	response := <-client.RouteDiameterRequest(&request, time.Duration(1000*time.Millisecond))
	switch v := response.(type) {
	case *diamcodec.DiameterMessage:
		fmt.Println(v)
	case error:
		fmt.Println(v)
	}
}

// Helper to navigate through peers
func findPeer(diameterHost string, table instrumentation.DiameterPeersTable) instrumentation.DiameterPeersTableEntry {
	for _, tableEntry := range table {
		if tableEntry.DiameterHost == diameterHost {
			return tableEntry
		}
	}

	return instrumentation.DiameterPeersTableEntry{}

}

// Helper to show JSON to humans
func PrettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
