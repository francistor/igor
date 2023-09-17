package router

import (
	"bytes"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/httphandler"
)

// This message handler parses the Igor1-Command, which may specify
// whether to introduce a small delay (value "Slow") or a big one (value "VerySlow")
// A User-Name attribute with the value "TestUserNameEcho" is added to the answer
func localDiameterHandler(request *core.DiameterMessage) (*core.DiameterMessage, error) {
	answer := core.NewDiameterAnswer(request)
	answer.Add("User-Name", "EchoLocal")
	answer.Add("Result-Code", core.DIAMETER_SUCCESS)

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

// The most basic handler ever. Returns an empty response to the received message
func localRadiusHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {
	hl := core.NewHandlerLogger()
	l := hl.L

	defer func(l *core.HandlerLogger) {
		l.WriteLog()
	}(hl)

	l.Infof("started localRadiusHandler for request %s", request)
	resp := core.NewRadiusResponse(request, true)
	resp.Add("User-Name", "EchoLocal")
	resp.Add("Tunnel-Password", "the password for the tunnel:1")

	command := request.GetStringAVP("Igor-Command")
	switch command {
	case "Slow":
		// Simulate the answer takes some time
		time.Sleep(300 * time.Millisecond)
	case "VerySlow":
		// Simulate the answer takes more time
		time.Sleep(5000 * time.Millisecond)
	}

	return resp, nil
}

func httpDiameterHandler(request *core.DiameterMessage) (*core.DiameterMessage, error) {
	answer := core.NewDiameterAnswer(request)
	answer.Add("Result-Code", core.DIAMETER_SUCCESS)
	answer.Add("User-Name", "EchoHTTP")

	return answer, nil
}

// The most basic handler ever. Returns an empty response to the received message
func httpRadiusHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {
	resp := core.NewRadiusResponse(request, true)
	resp.Add("User-Name", "EchoHTTP")

	return resp, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	core.InitPolicyConfigInstance("resources/searchRules.json", "testServer", nil, true)
	core.InitPolicyConfigInstance("resources/searchRules.json", "testClient", nil, false)
	core.InitPolicyConfigInstance("resources/searchRules.json", "testSuperServer", nil, false)
	core.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownClient", nil, false)
	core.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownServer", nil, false)
	core.InitHttpHandlerConfigInstance("resources/searchRules.json", "testServer", nil, false)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterBasicSetup(t *testing.T) {

	/* List of diameter servers and its peers

	client.igorclient			server.igorserver		superserver.igorsuperserver
	-----------------			----------------		---------------------------
	server.igorserver			client.igorclient		server.igorserver
	unreachable.igorserver

	unkclient.igorclient	(testClientUnknownClient)
	-------------------
	server.igorserver

	client.igorclient		(testClientUnknownServer)
	-----------------
	unkserver.igorserver

	*/

	// First start client, that will not be able to connect to the server (not instantiated).
	// Will retry the connection every second as per diameterServer.json configuration
	clientRouter := NewDiameterRouter("testClient", localDiameterHandler).Start()
	// Force some errors not being able to connect to server. Should recover later
	time.Sleep(2000 * time.Millisecond)

	superServerRouter := NewDiameterRouter("testSuperServer", localDiameterHandler).Start()
	time.Sleep(150 * time.Millisecond)
	serverRouter := NewDiameterRouter("testServer", localDiameterHandler).Start()

	time.Sleep(1100 * time.Millisecond)

	// Bad peers
	// This sleep time is important. Otherwise another client presenting himself
	// as client.igor generates a race condition and none of the client.igor
	// peers gets engaged
	time.Sleep(100 * time.Millisecond)
	b1 := NewDiameterRouter("testClientUnknownClient", localDiameterHandler).Start()
	b2 := NewDiameterRouter("testClientUnknownServer", localDiameterHandler).Start()

	// Time to settle connections
	time.Sleep(200 * time.Millisecond)

	// Uncomment to debug
	/*
		j, _ := json.Marshal(core.MS.PeersTableQuery())
		fmt.Println(PrettyPrintJSON(j))
	*/

	// Get the current peer status
	peerTables := core.IS.PeersTableQuery()

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
	serverRouter.Close()
	t.Log("Server Router terminated")

	superServerRouter.Close()
	t.Log("SuperServer Router terminated")

	clientRouter.Close()
	t.Log("Client Router terminated")

	b1.Close()

	b2.Close()
}

// Client will send message to Server, which will handle using http
// The two types of routes are tested here: to remote peer and to http handler
func TestDiameterRouteMessagetoHTTP(t *testing.T) {

	// Start handler
	httpHandler := httphandler.NewHttpHandler("testServer", httpDiameterHandler, httpRadiusHandler)
	time.Sleep(150 * time.Millisecond)

	// Start Routers
	server := NewDiameterRouter("testServer", localDiameterHandler).Start()
	time.Sleep(150 * time.Millisecond)
	client := NewDiameterRouter("testClient", localDiameterHandler).Start()

	// Some time to settle
	time.Sleep(300 * time.Millisecond)

	// Build request
	request, err := core.NewDiameterRequest("TestApplication", "TestRequest")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(core.GetPolicyConfig())
	request.Add("Destination-Realm", "igorserver")
	request.Add("User-Name", "TestUserNameRequest")
	response, err := client.RouteDiameterRequest(request, time.Duration(1000*time.Millisecond))
	if err != nil {
		t.Fatalf("route message returned error %s", err)
	} else if response.GetIntAVP("Result-Code") != core.DIAMETER_SUCCESS {
		t.Fatalf("Result-Code not succes %d", response.GetIntAVP("Result-Code"))
	} else if response.GetStringAVP("User-Name") != "EchoHTTP" {
		t.Fatalf("Echoed User-Name incorrect %s", response.GetStringAVP("User-Name"))
	}

	time.Sleep(100 * time.Millisecond)

	val, err := core.GetMetricWithLabels("http_client_exchanges", `{.*}`)
	if err != nil {
		t.Fatalf("error getting http_client_exchanges %s", err)
	}
	if val != "1" {
		t.Fatalf("number of http_client_exchanges messages was not 1")
	}
	val, err = core.GetMetricWithLabels("http_handler_exchanges", `{.*}`)
	if err != nil {
		t.Fatalf("error getting http_handler_exchanges %s", err)
	}
	if val != "1" {
		t.Fatalf("number of http_handler_exchanges messages was not 1")
	}

	client.Close()
	server.Close()

	httpHandler.Close()
}

// Client sends to server, which sends to superserver, which handles locally
func TestDiameterRouteMessagetoLocal(t *testing.T) {

	// Start Routers
	superServer := NewDiameterRouter("testSuperServer", localDiameterHandler).Start()
	server := NewDiameterRouter("testServer", nil).Start()
	time.Sleep(150 * time.Millisecond)
	client := NewDiameterRouter("testClient", nil).Start()

	// Some time to settle
	time.Sleep(300 * time.Millisecond)

	// Build request
	request, err := core.NewDiameterRequest("Gx", "Credit-Control")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(core.GetPolicyConfig())
	request.Add("Destination-Realm", "igorsuperserver")
	response, err := client.RouteDiameterRequest(request, time.Duration(1000*time.Millisecond))
	if err != nil {
		t.Fatalf("route message returned error %s", err)
	} else if response.GetIntAVP("Result-Code") != core.DIAMETER_SUCCESS {
		t.Fatalf("Result-Code not success %d", response.GetIntAVP("Result-Code"))
	} else if response.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("Echoed User-Name incorrect %s", response.GetStringAVP("User-Name"))
	}

	superServer.Close()
	server.Close()
	client.Close()

}

// Notice that http2 and local handlers do not get cancelled upon router termination
// and are not waited
func TestDiameterRequestCancellation(t *testing.T) {
	server := NewDiameterRouter("testServer", localDiameterHandler).Start()
	superserver := NewDiameterRouter("testSuperServer", localDiameterHandler).Start()

	time.Sleep(300 * time.Millisecond)

	// Build request that will be sent to superserver
	request, err := core.NewDiameterRequest("NASREQ", "AA")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(core.GetPolicyConfig())
	request.Add("Destination-Realm", "igorsuperserver")
	request.Add("Igor-Command", "VerySlow")

	var handlerCalled int32
	server.RouteDiameterRequestAsync(request, 200*time.Second, func(m *core.DiameterMessage, err error) {
		if err != nil {
			atomic.StoreInt32(&handlerCalled, 1)
		}
	})

	time.Sleep(100 * time.Millisecond)

	server.Close()

	// Give sometime for the async handler to execute
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&handlerCalled) != int32(1) {
		t.Fatalf("async handler was not called on router cancellation %d", atomic.LoadInt32(&handlerCalled))
	}

	superserver.Close()
}

func TestRouteParamRadiusPacket(t *testing.T) {
	rrouter := NewRadiusRouter("testServer", httpRadiusHandler).Start()

	rrouter.buildRadiusServersTable()

	rchan := make(chan interface{}, 1)
	req := RoutableRadiusRequest{
		Destination:       "igor-server-ne-group",
		Packet:            core.NewRadiusRequest(core.ACCESS_REQUEST),
		RChan:             rchan,
		PerRequestTimeout: 1 * time.Second,
		Tries:             3, // 1 will go to ne-server, 2 will go to igor-server, 3 will go again to ne-server
	}
	reqParams := rrouter.getRouteParams(req)
	if reqParams[2].endpoint != "127.0.0.2:51812" {
		t.Errorf("third try has wrong endpoint")
	}

	rrouter.Close()
}

// Client --> radiusPacket to --> Server --> httpHandler
func TestRadiusRouteToHTTP(t *testing.T) {
	// Start handler
	httpHandler := httphandler.NewHttpHandler("testServer", httpDiameterHandler, httpRadiusHandler)
	time.Sleep(150 * time.Millisecond)

	// Start Routers
	server := NewRadiusRouter("testServer", localRadiusHandler).Start()
	time.Sleep(150 * time.Millisecond)
	client := NewRadiusRouter("testClient", localRadiusHandler).Start()

	// Generate request
	req := core.NewRadiusRequest(core.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// Send to named group
	resp, err := client.RouteRadiusRequest(req, "igor-server-group", 2*time.Second, 1, 1, "secret")
	if err != nil {
		t.Fatalf("error sending request to igor-server-group %s", err)
	}
	if resp.GetStringAVP("User-Name") != "EchoHTTP" {
		t.Fatalf("bad response from server igor-server-group. Got %s", resp.GetStringAVP("User-Name"))
	}

	// Send to specific endpoint
	// Send to named group
	resp, err = client.RouteRadiusRequest(req, "127.0.0.1:1812", 2*time.Second, 1, 1, "secret")
	if err != nil {
		t.Fatalf("error sending request to 127.0.0.1:1812: %s", err)
	}
	if resp.GetStringAVP("User-Name") != "EchoHTTP" {
		t.Fatalf("bad response from server 127.0.0.1:1812. Got %s", resp.GetStringAVP("User-Name"))
	}

	val, err := core.GetMetricWithLabels("radius_client_requests", `{.*endpoint="127.0.0.1:1812"}`)
	if err != nil {
		t.Fatalf("error getting radius_client_requests %s", err)
	}
	if val != "2" {
		t.Fatalf("number of radius_client_requests messages was not 2")
	}

	client.Close()
	server.Close()

	httpHandler.Close()

}

func TestRadiusHandleLocal(t *testing.T) {

	// Start Routers
	client := NewRadiusRouter("testClient", localRadiusHandler).Start()

	// Generate request
	req := core.NewRadiusRequest(core.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// No destination: handle locally
	resp, err := client.RouteRadiusRequest(req, "", 2*time.Second, 1, 1, "")
	if err != nil {
		t.Fatalf("error sending request to testClient %s", err)
	}
	if resp.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("bad response from server testClient. Got %s", resp.GetStringAVP("User-Name"))
	}
	if resp.GetTaggedStringAVP("Tunnel-Password") != "the password for the tunnel:1" {
		t.Fatalf("bad response from server testClient. Got %s", resp.GetTaggedStringAVP("Tunnel-Password"))
	}

	client.Close()
}

func TestRadiusTimeout(t *testing.T) {

	core.IS.ResetMetrics()

	// Start handler
	httpHandler := httphandler.NewHttpHandler("testServer", httpDiameterHandler, httpRadiusHandler)
	time.Sleep(50 * time.Millisecond)

	// Start Routers
	superserver := NewRadiusRouter("testSuperServer", localRadiusHandler).Start()
	time.Sleep(50 * time.Millisecond)
	server := NewRadiusRouter("testServer", localRadiusHandler).Start()

	// Generate request
	req := core.NewRadiusRequest(core.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// Send to first server of named group (non existing) twice
	_, err := server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatalf("request did not get a timeout %s", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Two packets will be sent. Server not in quarantine
	serverTable := core.IS.RadiusServersTableQuery()
	if !findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is not available")
	}

	val, err := core.GetMetricWithLabels("radius_client_requests", `{.*endpoint="127.0.0.2:51812".*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_requests %s", err)
	}
	if val != "2" {
		t.Fatalf("number of radius_client_requests messages to non existing endpoint was not 2")
	}
	val, err = core.GetMetricWithLabels("radius_client_timeouts", `{.*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_timeouts %s", err)
	}
	if val != "2" {
		t.Fatalf("number of radius_client_timeouts messages was not 1")
	}

	// Repeat
	_, err = server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatalf("request did not get a timeout %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	// Repeat. Four packets will be reported as sent. Sever in quarantine
	serverTable = core.IS.RadiusServersTableQuery()
	if findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is available")
	}
	val, err = core.GetMetricWithLabels("radius_client_requests", `{.*endpoint="127.0.0.2:51812".*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_requests %s", err)
	}
	if val != "4" {
		t.Fatalf("number of radius_client_requests messages was not 1")
	}
	val, err = core.GetMetricWithLabels("radius_client_timeouts", `{.*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_timeouts %s", err)
	}
	if val != "4" {
		t.Fatalf("number of radius_client_timeouts messages was not 1")
	}

	// Repeat. Request will not get a timeout and the request will go to 11812
	_, err = server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err != nil {
		t.Fatalf("request failed %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	serverTable = core.IS.RadiusServersTableQuery()
	if findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is available")
	}

	val, err = core.GetMetricWithLabels("radius_client_requests", `{.*endpoint="127.0.0.2:51812".*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_requests %s", err)
	}
	if val != "4" {
		t.Fatalf("number of radius_client_requests messages was not 4")
	}
	val, err = core.GetMetricWithLabels("radius_client_requests", `{.*endpoint="127.0.0.1:11812".*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_requests %s", err)
	}
	if val != "1" {
		t.Fatalf("number of radius_client_requests messages was not 4")
	}
	val, err = core.GetMetricWithLabels("radius_client_timeouts", `{.*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_timeouts %s", err)
	}
	if val != "4" {
		t.Fatalf("number of radius_client_timeouts messages was not 1")
	}
	val, err = core.GetMetricWithLabels("radius_server_requests", `{.*endpoint="127.0.0.1".*}`)
	if err != nil {
		t.Fatalf("error getting radius_server_requests %s", err)
	}
	if val != "1" {
		t.Fatalf("number of radius_server_requests messages was not 5")
	}
	val, err = core.GetMetricWithLabels("radius_server_responses", `{.*endpoint="127.0.0.1".*}`)
	if err != nil {
		t.Fatalf("error getting radius_server_responses %s", err)
	}
	if val != "1" {
		t.Fatalf("number of radius_server_responses messages was not 5")
	}

	// Send to specific server
	_, err = server.RouteRadiusRequest(req, "127.0.0.1:7777", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatal("should get a timeout sending to non existing specific server")
	}

	val, err = core.GetMetricWithLabels("radius_client_timeouts", `{.*endpoint="127.0.0.1:7777".*}`)
	if err != nil {
		t.Fatalf("error getting radius_client_timeouts %s", err)
	}
	if val != "2" {
		t.Fatalf("number of radius_client_timeouts messages was not 6")
	}

	time.Sleep(1 * time.Second)

	superserver.Close()
	server.Close()

	httpHandler.Close()

}

func TestRadiusRequestCancellation(t *testing.T) {

	// Start Routers
	client := NewRadiusRouter("testClient", localRadiusHandler).Start()

	// Generate request
	req := core.NewRadiusRequest(core.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// Send the packet nowhere
	var handlerCalled int32
	client.RouteRadiusRequestAsync("127.0.0.1:7777", req, 200*time.Second, 1, 1, "", func(resp *core.RadiusPacket, err error) {
		if err != nil {
			atomic.StoreInt32(&handlerCalled, 1)
		}
	})

	time.Sleep(100 * time.Millisecond)

	client.Close()

	// Give some time for the async handler to execute
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&handlerCalled) != int32(1) {
		t.Fatalf("async handler was not called on router cancellation %d", atomic.LoadInt32(&handlerCalled))
	}
}

///////////////////////////////////////////////////////////////////////////////////

// Helper to navigate through peers
func findPeer(diameterHost string, table core.DiameterPeersTable) core.DiameterPeersTableEntry {
	for _, tableEntry := range table {
		if tableEntry.DiameterHost == diameterHost {
			return tableEntry
		}
	}

	return core.DiameterPeersTableEntry{}
}

func findRadiusServer(serverName string, table core.RadiusServersTable) core.RadiusServerTableEntry {
	for _, tableEntry := range table {
		if tableEntry.ServerName == serverName {
			return tableEntry
		}
	}

	return core.RadiusServerTableEntry{}
}

// Helper to show JSON to humans
func PrettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
