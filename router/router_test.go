package router

import (
	"bytes"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/diamcodec"
	"github.com/francistor/igor/httphandler"
	"github.com/francistor/igor/instrumentation"
	"github.com/francistor/igor/radiuscodec"
)

// This message handler parses the Igor1-Command, which may specify
// whether to introduce a small delay (value "Slow") or a big one (value "VerySlow")
// A User-Name attribute with the value "TestUserNameEcho" is added to the answer
func localDiameterHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	answer.Add("User-Name", "EchoLocal")
	answer.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)

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
func localRadiusHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	hl := config.NewHandlerLogger()
	l := hl.L

	defer func(l *config.HandlerLogger) {
		l.WriteLog()
	}(hl)

	l.Infof("started localRadiusHandler for request %s", request)
	resp := radiuscodec.NewRadiusResponse(request, true)
	resp.Add("User-Name", "EchoLocal")

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

func httpDiameterHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	answer.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
	answer.Add("User-Name", "EchoHTTP")

	return answer, nil
}

// The most basic handler ever. Returns an empty response to the received message
func httpRadiusHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	resp := radiuscodec.NewRadiusResponse(request, true)
	resp.Add("User-Name", "EchoHTTP")

	return resp, nil
}

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitPolicyConfigInstance("resources/searchRules.json", "testServer", true)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testSuperServer", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownClient", false)
	config.InitPolicyConfigInstance("resources/searchRules.json", "testClientUnknownServer", false)
	config.InitHttpHandlerConfigInstance("resources/searchRules.json", "testServer", false)

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
	} else if response.GetStringAVP("User-Name") != "EchoHTTP" {
		t.Fatalf("Echoed User-Name incorrect %s", response.GetStringAVP("User-Name"))
	}

	time.Sleep(100 * time.Millisecond)
	cm := instrumentation.MS.HttpClientQuery("HttpClientExchanges", nil, []string{})
	if cm[instrumentation.HttpClientMetricKey{}] != 1 {
		t.Fatalf("Client Exchanges was not 1")
	}
	hm := instrumentation.MS.HttpHandlerQuery("HttpHandlerExchanges", nil, []string{})
	if hm[instrumentation.HttpHandlerMetricKey{}] != 1 {
		t.Fatalf("Handler Exchanges was not 1")
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
	request, err := diamcodec.NewDiameterRequest("Gx", "Credit-Control")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(config.GetPolicyConfig())
	request.Add("Destination-Realm", "igorsuperserver")
	response, err := client.RouteDiameterRequest(request, time.Duration(1000*time.Millisecond))
	if err != nil {
		t.Fatalf("route message returned error %s", err)
	} else if response.GetIntAVP("Result-Code") != diamcodec.DIAMETER_SUCCESS {
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
	request, err := diamcodec.NewDiameterRequest("NASREQ", "AA")
	if err != nil {
		t.Fatalf("NewDiameterRequest error %s", err)
	}
	request.AddOriginAVPs(config.GetPolicyConfig())
	request.Add("Destination-Realm", "igorsuperserver")
	request.Add("Igor-Command", "VerySlow")

	var handlerCalled int32
	server.RouteDiameterRequestAsync(request, 200*time.Second, func(m *diamcodec.DiameterMessage, err error) {
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
		Packet:            radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST),
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
	req := radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST)
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

	client.Close()
	server.Close()

	httpHandler.Close()
}

func TestRadiusHandleLocal(t *testing.T) {

	// Start Routers
	client := NewRadiusRouter("testClient", localRadiusHandler).Start()

	// Generate request
	req := radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// No destination: handle locally
	resp, err := client.RouteRadiusRequest(req, "", 2*time.Second, 1, 1, "")
	if err != nil {
		t.Fatalf("error sending request to testClient %s", err)
	}
	if resp.GetStringAVP("User-Name") != "EchoLocal" {
		t.Fatalf("bad response from server testClient. Got %s", resp.GetStringAVP("User-Name"))
	}

	client.Close()
}

func TestRadiusTimeout(t *testing.T) {

	instrumentation.MS.ResetMetrics()

	// Start handler
	httpHandler := httphandler.NewHttpHandler("testServer", httpDiameterHandler, httpRadiusHandler)
	time.Sleep(50 * time.Millisecond)

	// Start Routers
	superserver := NewRadiusRouter("testSuperServer", localRadiusHandler).Start()
	time.Sleep(50 * time.Millisecond)
	server := NewRadiusRouter("testServer", localRadiusHandler).Start()

	// Generate request
	req := radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// Send to first server of named group (non existing) twice
	_, err := server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatalf("request did not get a timeout %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	// Two packets will be sent. Server not in quarantine
	requestsSentMetrics := instrumentation.MS.RadiusQuery("RadiusClientRequests", nil, nil)
	if requestsSentMetrics[instrumentation.RadiusMetricKey{}] != 2 {
		t.Fatalf("bad number of packets sent (could be due to network unavailable) %d", requestsSentMetrics[instrumentation.RadiusMetricKey{}])
	}
	serverTable := instrumentation.MS.RadiusServersTableQuery()
	if !findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is not available")
	}
	timeoutMetrics := instrumentation.MS.RadiusQuery("RadiusClientTimeouts", nil, nil)
	if timeoutMetrics[instrumentation.RadiusMetricKey{}] != 2 {
		t.Fatal("bad number of timeouts (could be due to network unavailable)")
	}

	// Repeat
	_, err = server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatalf("request did not get a timeout %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	// Repeat. Four packets will be reported as sent. Sever in quarantine
	requestsSentMetrics = instrumentation.MS.RadiusQuery("RadiusClientRequests", nil, nil)
	if requestsSentMetrics[instrumentation.RadiusMetricKey{}] != 4 {
		t.Fatal("bad number of packets sent", err)
	}
	serverTable = instrumentation.MS.RadiusServersTableQuery()
	if findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is available")
	}
	timeoutMetrics = instrumentation.MS.RadiusQuery("RadiusClientTimeouts", nil, nil)
	if timeoutMetrics[instrumentation.RadiusMetricKey{}] != 4 {
		t.Fatal("bad number of timeouts")
	}

	// Repeat. Request will not get a timeout and will increment client requests by one
	_, err = server.RouteRadiusRequest(req, "igor-server-ne-group", 100*time.Millisecond, 1, 2, "secret")
	if err != nil {
		t.Fatalf("request failed %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	requestsSentMetrics = instrumentation.MS.RadiusQuery("RadiusClientRequests", nil, nil)
	if requestsSentMetrics[instrumentation.RadiusMetricKey{}] != 5 {
		t.Fatal("bad number of packets sent", err)
	}
	serverTable = instrumentation.MS.RadiusServersTableQuery()
	if findRadiusServer("non-existing-server", serverTable["testServer"]).IsAvailable {
		t.Fatal("non-existing-server is available")
	}
	timeoutMetrics = instrumentation.MS.RadiusQuery("RadiusClientTimeouts", nil, nil)
	if timeoutMetrics[instrumentation.RadiusMetricKey{}] != 4 {
		t.Fatal("bad number of timeouts")
	}
	serverRequestsMetrics := instrumentation.MS.RadiusQuery("RadiusServerRequests", nil, []string{"Endpoint"})
	if serverRequestsMetrics[instrumentation.RadiusMetricKey{Endpoint: "127.0.0.1"}] != 1 {
		t.Fatalf("bad number of server requests %v", serverRequestsMetrics)
	} else {
		t.Log(serverRequestsMetrics)
	}
	serverResponsesMetrics := instrumentation.MS.RadiusQuery("RadiusServerResponses", nil, []string{"Endpoint"})
	if serverResponsesMetrics[instrumentation.RadiusMetricKey{Endpoint: "127.0.0.1"}] != 1 {
		t.Fatalf("bad number of server responses %v", serverResponsesMetrics)
	}

	// Send to specific server
	_, err = server.RouteRadiusRequest(req, "127.0.0.1:7777", 100*time.Millisecond, 1, 2, "secret")
	if err == nil {
		t.Fatal("should get a timeout sending to non existing specific server")
	}
	timeoutMetrics = instrumentation.MS.RadiusQuery("RadiusClientTimeouts", nil, nil)
	if timeoutMetrics[instrumentation.RadiusMetricKey{}] != 6 {
		t.Fatal("bad number of timeouts")
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
	req := radiuscodec.NewRadiusRequest(radiuscodec.ACCESS_REQUEST)
	req.Add("User-Name", "myUserName")

	// Send the packet nowhere
	var handlerCalled int32
	client.RouteRadiusRequestAsync("127.0.0.1:7777", req, 200*time.Second, 1, 1, "", func(resp *radiuscodec.RadiusPacket, err error) {
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
func findPeer(diameterHost string, table instrumentation.DiameterPeersTable) instrumentation.DiameterPeersTableEntry {
	for _, tableEntry := range table {
		if tableEntry.DiameterHost == diameterHost {
			return tableEntry
		}
	}

	return instrumentation.DiameterPeersTableEntry{}
}

func findRadiusServer(serverName string, table instrumentation.RadiusServersTable) instrumentation.RadiusServerTableEntry {
	for _, tableEntry := range table {
		if tableEntry.ServerName == serverName {
			return tableEntry
		}
	}

	return instrumentation.RadiusServerTableEntry{}
}

// Helper to show JSON to humans
func PrettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
