package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/handlerfunctions"
	"igor/httphandler"
	"igor/instrumentation"
	"igor/radiuscodec"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitPolicyConfigInstance("resources/searchRules.json", "testServer", true)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testSuperServer", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownServer", false)
	config.InitHandlerConfigInstance("resources/searchRules.json", "testServer", false)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterBasicSetup(t *testing.T) {
	superServerRouter := NewRouter("testSuperServer")
	time.Sleep(150 * time.Millisecond)
	serverRouter := NewRouter("testServer")
	time.Sleep(150 * time.Millisecond)
	clientRouter := NewRouter("testClient")

	// Bad peers
	// This sleep time is important. Otherwise another client presenting himself
	// as client.igor generates a race condition and none of the client.igor
	// peers gets engaged
	time.Sleep(200 * time.Millisecond)
	NewRouter("testClientUnknownClient")
	NewRouter("testClientUnknownServer")

	// Time to settle connections
	time.Sleep(1 * time.Second)

	// Uncomment to debug
	/*
		j, _ := json.Marshal(instrumentation.MS.PeersTableQuery())
		fmt.Println(PrettyPrintJSON(j))
	*/

	// Get the current peer status
	peerTables := instrumentation.MS.PeersTableQuery()

	// The testClient Router will have an established connection
	// to server.igor but not one to unreachableserver.igor
	clientTable := peerTables["testClient"]
	clientPeerServer := findPeer("server.igorserver", clientTable)
	if clientPeerServer.IsEngaged != true {
		t.Error("server.igor not engaged in client peers table")
	}
	clientPeerBadServer := findPeer("unreachableserver.igorserver", clientTable)
	if clientPeerBadServer.IsEngaged != false {
		t.Error("unreachableserver.igor engaged in client peers table")
	}

	// The testServer Router will have two established connections
	// with client.igor and superserver.igorsuperserver
	serverTable := peerTables["testServer"]
	serverPeerClient := findPeer("client.igorclient", serverTable)
	if serverPeerClient.IsEngaged != true {
		t.Error("server.igor not engaged in server peers table")

	}
	serverPeerSuperServer := findPeer("superserver.igorsuperserver", serverTable)
	if serverPeerSuperServer.IsEngaged != true {
		t.Error("superserver.igorsuperserver not engaged in server peers table")
	}

	// The testSuperServer will have an established connection with
	// server.igor
	superserverTable := peerTables["testSuperServer"]
	superserverPeerServer := findPeer("server.igorserver", superserverTable)
	if superserverPeerServer.IsEngaged != true {
		t.Error("server.igorserver not engaged in superserverserver peers table")
	}

	// Bad clients
	// testClientUnknownClient tries to register with server.igorserver with
	// a Diameter-Host name that is not recognized by the server
	unkClientTable := peerTables["testClientUnknownClient"]
	clientPeerUnknownClient := findPeer("server.igorserver", unkClientTable)
	if clientPeerUnknownClient.IsEngaged != false {
		t.Error("server.igorserver engaged in unknownclient peers table")
	}

	// testClientUnknownServer tries to register with a server in the
	// same address where server.igor is lisening but expecting another
	// server name
	unkServerClientTable := peerTables["testClientUnknownServer"]
	clientPeerUnknownServer := findPeer("server.igorserver", unkServerClientTable)
	if clientPeerUnknownServer.IsEngaged != false {
		t.Error("server.igorserver engaged in unknownserver peers table")
	}

	// Close Routers
	serverRouter.SetDown()
	serverRouter.Close()
	t.Log("Server Router terminated")

	superServerRouter.SetDown()
	superServerRouter.Close()
	t.Log("SuperServer Router terminated")

	clientRouter.SetDown()
	clientRouter.Close()
	t.Log("Client Router terminated")

}

// Client will send message to Server, which will handle locally
// The two types of routes are tested here
func TestDiameterRouteMessage(t *testing.T) {

	// Start handler
	httphandler.NewHttpHandler("testServer", handlerfunctions.EmptyDiameterHandler, handlerfunctions.EmptyRadiusHandler)
	time.Sleep(150 * time.Millisecond)

	// Start Routers
	NewRouter("testServer")
	time.Sleep(150 * time.Millisecond)
	client := NewRouter("testClient")

	// Some time to settle
	time.Sleep(500 * time.Millisecond)

	// Build request
	request, err := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(config.GetPolicyConfig())
	request.Add("Destination-Realm", "igorsuperserver")
	request.Add("User-Name", "TestUserNameRequest")
	response, err := client.RouteDiameterRequest(request, time.Duration(1000*time.Millisecond))
	if err != nil {
		t.Fatalf("route message returned error %s", err)
	} else if response.GetIntAVP("Result-Code") != diamcodec.DIAMETER_SUCCESS {
		t.Fatalf("Result-Code not succes %d", response.GetIntAVP("Result-Code"))
	}

	time.Sleep(200 * time.Millisecond)
	cm := instrumentation.MS.HttpClientQuery("HttpClientExchanges", nil, []string{})
	if cm[instrumentation.HttpClientMetricKey{}] != 1 {
		t.Fatalf("Client Exchanges was not 1")
	}
	hm := instrumentation.MS.HttpHandlerQuery("HttpHandlerExchanges", nil, []string{})
	if hm[instrumentation.HttpHandlerMetricKey{}] != 1 {
		t.Fatalf("Handler Exchanges was not 1")
	}
}

func TestRouteRadiusPacket(t *testing.T) {
	rrouter := NewRadiusRouter("testServer", echoHandler)

	rrouter.updateRadiusServersTable()

	rchan := make(chan interface{}, 1)
	req := RoutableRadiusRequest{
		destination: "igor-server-ne-group",
		packet:      radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST),
		rchan:       rchan,
		timeout:     1 * time.Second,
		retries:     2,
	}
	reqParams := rrouter.getRouteParams(req)
	if reqParams[2].endpoint != "192.168.250.1:11812" {
		t.Errorf("third try has wrong endpoint")
	}
	fmt.Println(reqParams)

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

// Simple handler that generates a success response with the same attributes as in the request
func echoHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {

	response := radiuscodec.NewRadiusResponse(request, true)
	for i := range request.AVPs {
		response.AddAVP(&request.AVPs[i])
	}

	return response, nil
}
