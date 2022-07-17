package instrumentation

import (
	"igor/config"
	"igor/diamcodec"
	"os"
	"testing"
	"time"
)

// Initializer of the test suite.
func TestMain(m *testing.M) {

	// Initialization
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testClient"
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	diameterRequest, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	diameterRequest.AddOriginAVPs(config.GetPolicyConfig())
	diameterAnswer := diamcodec.NewDiameterAnswer(diameterRequest)
	diameterAnswer.AddOriginAVPs(config.GetPolicyConfig())

	// Generate some metrics
	PushPeerDiameterRequestReceived("testPeer", diameterRequest)
	PushPeerDiameterRequestSent("testPeer", diameterRequest)
	PushPeerDiameterRequestTimeout("testPeer", PeerDiameterMetricFromMessage("testPeer", diameterRequest))
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

func TestHttpClientMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	PushHttpClientExchange("https://localhost", "200")
	PushHttpClientExchange("https://localhost", "200")
	PushHttpHandlerExchange("500")
	PushHttpHandlerExchange("300")
	PushHttpHandlerExchange("300")

	time.Sleep(100 * time.Millisecond)

	// Check Http Client Metrics
	cm := MS.HttpClientQuery("HttpClientExchanges", nil, []string{"Endpoint"})
	if v, ok := cm[HttpClientMetricKey{Endpoint: "https://localhost"}]; !ok {
		t.Fatalf("HttpClientExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpClientExchanges is not 2")
	}

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
}
