package instrumentation

import (
	"os"
	"testing"
	"time"

	"github.com/francistor/igor/core"
)

// Initializer of the test suite.
func TestMain(m *testing.M) {

	// Initialization
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testClient"
	core.InitPolicyConfigInstance(bootstrapFile, instanceName, true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	diameterRequest, _ := core.NewDiameterRequest("TestApplication", "TestRequest")
	diameterRequest.AddOriginAVPs(core.GetPolicyConfig())
	diameterAnswer := core.NewDiameterAnswer(diameterRequest)
	diameterAnswer.AddOriginAVPs(core.GetPolicyConfig())

	// Generate some metrics
	PushPeerDiameterRequestReceived("testPeer", diameterRequest)
	PushPeerDiameterRequestSent("testPeer", diameterRequest)
	PushPeerDiameterRequestTimeout(PeerDiameterMetricFromMessage("testPeer", diameterRequest))
	PushPeerDiameterAnswerReceived("testPeer", diameterAnswer)
	PushPeerDiameterAnswerSent("testPeer", diameterAnswer)
	PushPeerDiameterAnswerStalled("testPeer", diameterAnswer)
	PushRouterRouteNotFound("testPeer", diameterRequest)
	PushRouterHandlerError("testPeer", diameterRequest)
	PushRouterNoAvailablePeer("testPeer", diameterRequest)

	time.Sleep(100 * time.Millisecond)

	// Check Metrics
	dm := MS.DiameterQuery("DiameterRequestsReceived", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsReceived not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsReceived is not 1")
	}
	dm = MS.DiameterQuery("DiameterRequestsSent", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsSent not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsSent is not 1")
	}
	dm = MS.DiameterQuery("DiameterAnswersReceived", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersReceived not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersReceived is not 1")
	}
	dm = MS.DiameterQuery("DiameterAnswersSent", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersSent not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersSent is not 1")
	}
	dm = MS.DiameterQuery("DiameterRequestsTimeout", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsTimeout not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsTimeout is not 1")
	}
	dm = MS.DiameterQuery("DiameterAnswersStalled", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersStalled not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersStalled is not 1")
	}
	dm = MS.DiameterQuery("DiameterRouteNotFound", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRouteNotFound not found")
	} else if v != 1 {
		t.Fatalf("DiameterRouteNotFound is not 1")
	}
	dm = MS.DiameterQuery("DiameterNoAvailablePeer", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterNoAvailablePeer not found")
	} else if v != 1 {
		t.Fatalf("DiameterNoAvailablePeer is not 1")
	}
	dm = MS.DiameterQuery("DiameterHandlerError", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterHandlerError not found")
	} else if v != 1 {
		t.Fatalf("DiameterHandlerError is not 1")
	}

	dm = MS.DiameterQuery("NonExistingMetric", nil, []string{"Peer"})
	if _, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; ok {
		t.Fatalf("NonExistingMetric found!")
	}
}

func TestHttpMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	PushHttpClientExchange("https://localhost", "200")
	PushHttpClientExchange("https://localhost", "200")

	PushHttpHandlerExchange("500", "/DiameterRequest")
	PushHttpHandlerExchange("300", "/DiameterRequest")
	PushHttpHandlerExchange("300", "/RadiusRequest")

	PushHttpRouterExchange("200", "/routeRadiusRequest")
	PushHttpRouterExchange("200", "/routeDiameterRequest")
	PushHttpRouterExchange("300", "/routeDiameterRequest")

	time.Sleep(100 * time.Millisecond)

	// Check Http Client Metrics
	cm := MS.HttpClientQuery("HttpClientExchanges", nil, []string{"Endpoint"})
	if v, ok := cm[HttpClientMetricKey{Endpoint: "https://localhost"}]; !ok {
		t.Fatalf("HttpClientExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpClientExchanges is not 2")
	}

	// Check Http Handler Metrics
	hm1 := MS.HttpHandlerQuery("HttpHandlerExchanges", nil, []string{"ErrorCode"})
	if v, ok := hm1[HttpHandlerMetricKey{ErrorCode: "300"}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpHandlerExchanges is not 2")
	}

	hm2 := MS.HttpHandlerQuery("HttpHandlerExchanges", nil, []string{})
	if v, ok := hm2[HttpHandlerMetricKey{}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 3 {
		t.Fatalf("HttpHandlerExchanges is not 3")
	}

	hm3 := MS.HttpHandlerQuery("HttpHandlerExchanges", nil, []string{"Path"})
	if v, ok := hm3[HttpHandlerMetricKey{Path: "/DiameterRequest"}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpHandlerExchanges is not 1")
	}

	// Check Http Router Metrics
	rm1 := MS.HttpRouterQuery("HttpRouterExchanges", nil, []string{"Path"})
	if v, ok := rm1[HttpRouterMetricKey{Path: "/routeDiameterRequest"}]; !ok {
		t.Fatalf("HttpRouterExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpRouterExchanges is not 2")
	}
}

func TestRadiusMetrics(t *testing.T) {
	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	PushRadiusServerRequest("127.0.0.1:1812", "1")
	PushRadiusServerResponse("127.0.0.1:1812", "2")
	PushRadiusServerDrop("127.0.0.1:1812", "1")
	PushRadiusClientRequest("127.0.0.1:1812", "1")
	PushRadiusClientResponse("127.0.0.1:1812", "2")
	PushRadiusClientTimeout("127.0.0.1:1812", "1")
	PushRadiusClientResponseStalled("127.0.0.1:1812", "1")
	PushRadiusClientResponseDrop("127.0.0.1:1812", "1")

	time.Sleep(100 * time.Millisecond)
	rm := MS.RadiusQuery("RadiusServerRequests", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerRequests not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerRequests is not 1")
	}
	rm = MS.RadiusQuery("RadiusServerResponses", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerResponses not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerResponses is not 1")
	}
	rm = MS.RadiusQuery("RadiusServerDrops", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerDrops not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerDrops is not 1")
	}

	rm = MS.RadiusQuery("RadiusClientRequests", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientsRequests not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientsRequests is not 1")
	}
	rm = MS.RadiusQuery("RadiusClientResponses", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponses not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponses is not 1")
	}
	rm = MS.RadiusQuery("RadiusClientTimeouts", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientTimeouts not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientTimeouts is not 1")
	}
	rm = MS.RadiusQuery("RadiusClientResponsesStalled", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponsesStalled not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponsesStalled is not 1")
	}
	rm = MS.RadiusQuery("RadiusClientResponsesDrop", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponsesDrop not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponsesDrop is not 1")
	}

}
