package core

import (
	"strings"
	"testing"
	"time"
)

func TestDiameterMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	diameterRequest, _ := NewDiameterRequest("TestApplication", "TestRequest")
	diameterRequest.AddOriginAVPs(GetPolicyConfig())
	diameterAnswer := NewDiameterAnswer(diameterRequest)
	diameterAnswer.AddOriginAVPs(GetPolicyConfig())

	// Generate some metrics
	IncrementPeerDiameterRequestReceived("testPeer", diameterRequest)
	IncrementPeerDiameterRequestSent("testPeer", diameterRequest)
	IncrementPeerDiameterRequestTimeout(PeerDiameterMetricFromMessage("testPeer", diameterRequest))
	IncrementPeerDiameterAnswerReceived("testPeer", diameterAnswer)
	IncrementPeerDiameterAnswerSent("testPeer", diameterAnswer)
	IncrementPeerDiameterAnswerStalled("testPeer", diameterAnswer)
	IncrementRouterRouteNotFound("testPeer", diameterRequest)
	IncrementRouterHandlerError("testPeer", diameterRequest)
	IncrementRouterNoAvailablePeer("testPeer", diameterRequest)

	time.Sleep(100 * time.Millisecond)

	// Check Metrics
	dm := MetricQuery[PeerDiameterMetrics](MS, "DiameterRequestsReceived", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsReceived not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsReceived is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterRequestsSent", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsSent not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsSent is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterAnswersReceived", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersReceived not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersReceived is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterAnswersSent", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersSent not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersSent is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterRequestsTimeout", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRequestsTimeout not found")
	} else if v != 1 {
		t.Fatalf("DiameterRequestsTimeout is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterAnswersStalled", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterAnswersStalled not found")
	} else if v != 1 {
		t.Fatalf("DiameterAnswersStalled is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterRouteNotFound", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterRouteNotFound not found")
	} else if v != 1 {
		t.Fatalf("DiameterRouteNotFound is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterNoAvailablePeer", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterNoAvailablePeer not found")
	} else if v != 1 {
		t.Fatalf("DiameterNoAvailablePeer is not 1")
	}
	dm = MetricQuery[PeerDiameterMetrics](MS, "DiameterHandlerError", nil, []string{"Peer"})
	if v, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; !ok {
		t.Fatalf("DiameterHandlerError not found")
	} else if v != 1 {
		t.Fatalf("DiameterHandlerError is not 1")
	}

	dm = MetricQuery[PeerDiameterMetrics](MS, "NonExistingMetric", nil, []string{"Peer"})
	if _, ok := dm[PeerDiameterMetricKey{Peer: "testPeer"}]; ok {
		t.Fatalf("NonExistingMetric found!")
	}
}

func TestHttpMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	IncrementHttpClientExchange("https://localhost", "200")
	IncrementHttpClientExchange("https://localhost", "200")

	IncrementHttpHandlerExchange("/DiameterRequest", "500")
	IncrementHttpHandlerExchange("/DiameterRequest", "300")
	IncrementHttpHandlerExchange("/RadiusRequest", "300")

	IncrementHttpRouterExchange("/routeRadiusRequest", "200")
	IncrementHttpRouterExchange("/routeDiameterRequest", "200")
	IncrementHttpRouterExchange("/routeDiameterRequest", "300")

	time.Sleep(100 * time.Millisecond)

	// Check Http Client Metrics
	cm := MetricQuery[HttpClientMetrics](MS, "HttpClientExchanges", nil, []string{"Endpoint"})
	if v, ok := cm[HttpClientMetricKey{Endpoint: "https://localhost"}]; !ok {
		t.Fatalf("HttpClientExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpClientExchanges is not 2")
	}

	// Check Http Handler Metrics
	hm1 := MetricQuery[HttpHandlerMetrics](MS, "HttpHandlerExchanges", nil, []string{"ErrorCode"})
	if v, ok := hm1[HttpHandlerMetricKey{ErrorCode: "300"}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpHandlerExchanges is not 2")
	}

	hm2 := MetricQuery[HttpHandlerMetrics](MS, "HttpHandlerExchanges", nil, []string{})
	if v, ok := hm2[HttpHandlerMetricKey{}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 3 {
		t.Fatalf("HttpHandlerExchanges is not 3")
	}

	hm3 := MetricQuery[HttpHandlerMetrics](MS, "HttpHandlerExchanges", nil, []string{"Path"})
	if v, ok := hm3[HttpHandlerMetricKey{Path: "/DiameterRequest"}]; !ok {
		t.Fatalf("HttpHandlerExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpHandlerExchanges is not 1")
	}

	// Check Http Router Metrics
	rm1 := MetricQuery[HttpRouterMetrics](MS, "HttpRouterExchanges", nil, []string{"Path"})
	if v, ok := rm1[HttpRouterMetricKey{Path: "/routeDiameterRequest"}]; !ok {
		t.Fatalf("HttpRouterExchanges not found")
	} else if v != 2 {
		t.Fatalf("HttpRouterExchanges is not 2")
	}
}

func TestSessionServerMetrics(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	IncrementSessionQueries("/sessionserver/v1/sessions", "Framed-IP-Address", "200")
	IncrementSessionQueries("/sessionserver/v1/sessions", "Framed-IP-Address", "200")
	IncrementSessionQueries("/sessionserver/v1/sessions", "Bad-Attribute", "400")

	IncrementSessionUpdateQueries("1.1.1.1")
	IncrementSessionUpdateQueries("1.1.1.1")
	IncrementSessionUpdateQueries("2.2.2.2")

	time.Sleep(100 * time.Millisecond)

	// Check SessionQuery Metrics
	sq := MetricQuery[SessionQueryMetrics](MS, "SessionQueries", nil, []string{"Path", "IndexName"})
	if v, ok := sq[SessionQueryMetricKey{Path: "/sessionserver/v1/sessions", IndexName: "Framed-IP-Address"}]; !ok {
		t.Fatalf("SessionQueries not found")
	} else if v != 2 {
		t.Fatalf("SessionQueries is not 2")
	}

	sq1 := MetricQuery[SessionQueryMetrics](MS, "SessionQueries", nil, []string{})
	if v, ok := sq1[SessionQueryMetricKey{}]; !ok {
		t.Fatalf("SessionQueries not found")
	} else if v != 3 {
		t.Fatalf("SessionQueries is not 3")
	}

	// Check Session Update Metrics
	su := MetricQuery[SessionUpdateMetrics](MS, "SessionUpdates", nil, []string{"Endpoint"})
	if v, ok := su[SessionUpdateMetricKey{Endpoint: "1.1.1.1"}]; !ok {
		t.Fatalf("SessionUpdates not found")
	} else if v != 2 {
		t.Fatalf("SessionUpdates is not 2")
	}

	// Check Session Update Metrics
	su1 := MetricQuery[SessionUpdateMetrics](MS, "SessionUpdates", nil, []string{"Endpoint"})
	if v, ok := su1[SessionUpdateMetricKey{Endpoint: "1.1.1.1"}]; !ok {
		t.Fatalf("SessionUpdates not found (generic)")
	} else if v != 2 {
		t.Fatalf("SessionUpdates is not 2 (generic)")
	}
}

func TestRadiusMetrics(t *testing.T) {
	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	IncrementRadiusServerRequest("127.0.0.1:1812", "1")
	IncrementRadiusServerResponse("127.0.0.1:1812", "2")
	IncrementRadiusServerDrop("127.0.0.1:1812", "1")
	IncrementRadiusClientRequest("127.0.0.1:1812", "1")
	IncrementRadiusClientResponse("127.0.0.1:1812", "2")
	IncrementRadiusClientTimeout("127.0.0.1:1812", "1")
	IncrementRadiusClientResponseStalled("127.0.0.1:1812", "1")
	IncrementRadiusClientResponseDrop("127.0.0.1:1812", "1")

	time.Sleep(100 * time.Millisecond)
	rm := MetricQuery[RadiusMetrics](MS, "RadiusServerRequests", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerRequests not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerRequests is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusServerResponses", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerResponses not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerResponses is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusServerDrops", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusServerDrops not found")
	} else if v != 1 {
		t.Fatalf("RadiusServerDrops is not 1")
	}

	rm = MetricQuery[RadiusMetrics](MS, "RadiusClientRequests", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientsRequests not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientsRequests is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusClientResponses", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponses not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponses is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusClientTimeouts", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientTimeouts not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientTimeouts is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusClientResponsesStalled", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponsesStalled not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponsesStalled is not 1")
	}
	rm = MetricQuery[RadiusMetrics](MS, "RadiusClientResponsesDrops", nil, []string{"Endpoint"})
	if v, ok := rm[RadiusMetricKey{Endpoint: "127.0.0.1:1812"}]; !ok {
		t.Fatalf("RadiusClientResponsesDrops not found")
	} else if v != 1 {
		t.Fatalf("RadiusClientResponsesDrops is not 1")
	}
}

func TestPrometheusEndpoint(t *testing.T) {

	MS.ResetMetrics()
	time.Sleep(100 * time.Millisecond)

	diameterRequest, _ := NewDiameterRequest("TestApplication", "TestRequest")
	diameterRequest.AddOriginAVPs(GetPolicyConfig())
	diameterAnswer := NewDiameterAnswer(diameterRequest)
	diameterAnswer.AddOriginAVPs(GetPolicyConfig())

	// Generate some metrics
	IncrementPeerDiameterRequestReceived("testPeer", diameterRequest)
	IncrementPeerDiameterRequestSent("testPeer", diameterRequest)
	IncrementPeerDiameterRequestTimeout(PeerDiameterMetricFromMessage("testPeer", diameterRequest))
	IncrementPeerDiameterAnswerReceived("testPeer", diameterAnswer)
	IncrementPeerDiameterAnswerSent("testPeer", diameterAnswer)
	IncrementPeerDiameterAnswerStalled("testPeer", diameterAnswer)
	IncrementRouterRouteNotFound("testPeer", diameterRequest)
	IncrementRouterHandlerError("testPeer", diameterRequest)
	IncrementRouterNoAvailablePeer("testPeer", diameterRequest)

	IncrementHttpClientExchange("https://localhost", "200")
	IncrementHttpClientExchange("https://localhost", "200")

	IncrementHttpHandlerExchange("/DiameterRequest", "500")
	IncrementHttpHandlerExchange("/DiameterRequest", "300")
	IncrementHttpHandlerExchange("/RadiusRequest", "300")

	IncrementHttpRouterExchange("/routeRadiusRequest", "200")
	IncrementHttpRouterExchange("/routeDiameterRequest", "300")
	IncrementHttpRouterExchange("/routeDiameterRequest", "300")

	IncrementRadiusServerRequest("127.0.0.1:1812", "1")
	IncrementRadiusServerResponse("127.0.0.1:1812", "2")
	IncrementRadiusServerDrop("127.0.0.1:1812", "1")
	IncrementRadiusClientRequest("127.0.0.1:1812", "1")
	IncrementRadiusClientResponse("127.0.0.1:1812", "2")
	IncrementRadiusClientTimeout("127.0.0.1:1812", "1")
	IncrementRadiusClientResponseStalled("127.0.0.1:1812", "1")
	IncrementRadiusClientResponseDrop("127.0.0.1:1812", "1")

	IncrementSessionQueries("/sessionserver/v1/sessions", "Framed-IP-Address", "200")
	IncrementSessionQueries("/sessionserver/v1/sessions", "Framed-IP-Address", "200")
	IncrementSessionQueries("/sessionserver/v1/sessions", "Bad-Attribute", "400")

	IncrementSessionUpdateQueries("1.1.1.1")
	IncrementSessionUpdateQueries("1.1.1.1")
	IncrementSessionUpdateQueries("2.2.2.2")

	metrics, err := httpGet("http://localhost:9090/metrics")
	if err != nil {
		t.Fatalf("could not get metrics: %s", err)
	}

	if !strings.Contains(metrics, `diameter_requests_received{peer="testPeer",oh="server.igorserver",or="igorserver",dh="",dr="",ap="TestApplication",cm="TestRequest"} 1`) {
		t.Fatal("diameter_requests_received not found or incorrect")
	}
	if !strings.Contains(metrics, `radius_server_requests{endpoint="127.0.0.1:1812",code="1"} 1`) {
		t.Fatal("radius_server_requests not found or incorrect")
	}
	if !strings.Contains(metrics, `radius_client_requests{endpoint="127.0.0.1:1812",code="1"} 1`) {
		t.Fatal("radius_client_requests not found or incorrect")
	}
	if !strings.Contains(metrics, `session_queries{path="/sessionserver/v1/sessions",indexname="Framed-IP-Address",errorcode="200"} 2`) {
		t.Fatal("session_server_queries not found or incorrect")
	}
	if !strings.Contains(metrics, `session_updates{endpoint="1.1.1.1"} 2`) {
		t.Fatal("session_server_updates not found or incorrect")
	}

	// TODO: add others

	metricsJSON, _ := httpGet("http://localhost:9090/diameterMetrics/diameterRequestsReceived?agg=Peer&oh=server.igorserver")
	if !strings.Contains(metricsJSON, `"Peer":"testPeer"`) || !strings.Contains(metricsJSON, `"Value":1`) {
		t.Fatal("bad diameterMetric " + metricsJSON)
	}

	metricsJSON, _ = httpGet("http://localhost:9090/radiusMetrics/radiusServerRequests?agg=Code&endpoint=127.0.0.1:1812")
	if !strings.Contains(metricsJSON, `"Code":"1"`) || !strings.Contains(metricsJSON, `"Value":1`) {
		t.Fatal("bad radiusMetric " + metricsJSON)
	}

	// TODO: add others
}
