package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics to be used in the instrumented code
var pm struct {
	RadiusMetrics        *RadiusPrometheusMetrics
	DiameterMetrics      *DiameterPrometheusMetrics
	HttpClientMetrics    *HttpClientPrometheusMetrics
	HttpHandlerMetrics   *HttpHandlerPrometheusMetrics
	HttpRouterMetrics    *HttpRouterPrometheusMetrics
	SessionServerMetrics *SessionServerPrometheusMetrics
}

// ///////////////////////////////////////////////////////////////
// Metrics definitions
// ///////////////////////////////////////////////////////////////
type RadiusPrometheusMetrics struct {
	RadiusServerRequests         *prometheus.CounterVec
	RadiusServerResponses        *prometheus.CounterVec
	RadiusServerDrops            *prometheus.CounterVec
	RadiusClientRequests         *prometheus.CounterVec
	RadiusClientResponses        *prometheus.CounterVec
	RadiusClientTimeouts         *prometheus.CounterVec
	RadiusClientResponsesStalled *prometheus.CounterVec
	RadiusClientResponsesDropped *prometheus.CounterVec
}

func (m *RadiusPrometheusMetrics) reset() {
	m.RadiusServerRequests.Reset()
	m.RadiusServerResponses.Reset()
	m.RadiusServerDrops.Reset()
	m.RadiusClientRequests.Reset()
	m.RadiusClientResponses.Reset()
	m.RadiusClientTimeouts.Reset()
	m.RadiusClientResponsesStalled.Reset()
	m.RadiusClientResponsesDropped.Reset()
}

type DiameterPrometheusMetrics struct {
	PeerDiameterRequestsReceived *prometheus.CounterVec
	PeerDiameterAnswersSent      *prometheus.CounterVec
	PeerDiameterRequestsSent     *prometheus.CounterVec
	PeerDiameterAnswersReceived  *prometheus.CounterVec
	PeerDiameterRequestTimeouts  *prometheus.CounterVec
	PeerDiameterAnswersStalled   *prometheus.CounterVec

	RouterRoutesNotFound   *prometheus.CounterVec
	RouterPeerNotAvailable *prometheus.CounterVec

	RouterHandlerErrors *prometheus.CounterVec
}

func (m *DiameterPrometheusMetrics) reset() {
	m.PeerDiameterRequestsReceived.Reset()
	m.PeerDiameterAnswersSent.Reset()
	m.PeerDiameterRequestsSent.Reset()
	m.PeerDiameterAnswersReceived.Reset()
	m.PeerDiameterRequestTimeouts.Reset()
	m.PeerDiameterAnswersStalled.Reset()

	m.RouterRoutesNotFound.Reset()
	m.RouterPeerNotAvailable.Reset()

	m.RouterHandlerErrors.Reset()
}

type HttpClientPrometheusMetrics struct {
	HttpClientExchanges *prometheus.CounterVec
}

func (m *HttpClientPrometheusMetrics) reset() {
	m.HttpClientExchanges.Reset()
}

type HttpHandlerPrometheusMetrics struct {
	HttpHandlerExchanges *prometheus.CounterVec
}

func (m *HttpHandlerPrometheusMetrics) reset() {
	m.HttpHandlerExchanges.Reset()
}

type HttpRouterPrometheusMetrics struct {
	HttpRouterExchanges *prometheus.CounterVec
}

func (m *HttpRouterPrometheusMetrics) reset() {
	m.HttpRouterExchanges.Reset()
}

type SessionServerPrometheusMetrics struct {
	SessionQueries *prometheus.CounterVec
	SessionUpdates *prometheus.CounterVec
	SessionCount   *prometheus.GaugeVec
}

func (m *SessionServerPrometheusMetrics) reset() {
	m.SessionQueries.Reset()
	m.SessionUpdates.Reset()
}

func newRadiusPrometheusMetrics(reg prometheus.Registerer) *RadiusPrometheusMetrics {
	m := &RadiusPrometheusMetrics{

		RadiusServerRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_server_requests",
				Help: "Radius server requests",
			},
			[]string{"endpoint", "code"}),

		RadiusServerResponses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_server_responses",
				Help: "Radius server responses",
			},
			[]string{"endpoint", "code"}),

		RadiusServerDrops: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_server_drops",
				Help: "Radius server dropped packets",
			},
			[]string{"endpoint", "code"}),

		RadiusClientRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_client_requests",
				Help: "Radius client requests",
			},
			[]string{"endpoint", "code"}),

		RadiusClientResponses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_client_responses",
				Help: "Radius client responses",
			},
			[]string{"endpoint", "code"}),

		RadiusClientTimeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_client_timeouts",
				Help: "Radius client timeouts",
			},
			[]string{"endpoint", "code"}),

		RadiusClientResponsesStalled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_client_responses_stalled",
				Help: "Radius client responses stalled",
			},
			[]string{"endpoint", "code"}),

		RadiusClientResponsesDropped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "radius_client_responses_dropped",
				Help: "Radius client responses dropped",
			},
			[]string{"endpoint", "code"}),
	}

	reg.MustRegister(m.RadiusServerRequests)
	reg.MustRegister(m.RadiusServerResponses)
	reg.MustRegister(m.RadiusServerDrops)
	reg.MustRegister(m.RadiusClientRequests)
	reg.MustRegister(m.RadiusClientResponses)
	reg.MustRegister(m.RadiusClientTimeouts)
	reg.MustRegister(m.RadiusClientResponsesStalled)
	reg.MustRegister(m.RadiusClientResponsesDropped)

	return m
}

func newDiameterPrometheusMetrics(reg prometheus.Registerer) *DiameterPrometheusMetrics {
	m := &DiameterPrometheusMetrics{
		PeerDiameterRequestsReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_requests_received",
				Help: "Diameter requests received",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		PeerDiameterAnswersSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_answers_sent",
				Help: "Diameter answers sent",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		PeerDiameterRequestsSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_requests_sent",
				Help: "Diameter requests sent",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		PeerDiameterAnswersReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_answers_received",
				Help: "Diameter answers received",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		PeerDiameterRequestTimeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_request_timeouts",
				Help: "Diameter request timeouts",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		PeerDiameterAnswersStalled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "diameter_answers_stalled",
				Help: "Diameter answers_stalled",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		RouterRoutesNotFound: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "router_routes_not_found",
				Help: "Messages not sent due to diameter route not found",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		RouterPeerNotAvailable: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "router_peer_not_available",
				Help: "Messages not sent due to no peer available",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),

		RouterHandlerErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "router_handler_error",
				Help: "Errors in diameter handler",
			},
			[]string{"peer", "oh", "or", "dh", "dr", "ap", "cm"}),
	}

	reg.MustRegister(m.PeerDiameterRequestsReceived)
	reg.MustRegister(m.PeerDiameterAnswersSent)
	reg.MustRegister(m.PeerDiameterRequestsSent)
	reg.MustRegister(m.PeerDiameterAnswersReceived)
	reg.MustRegister(m.PeerDiameterRequestTimeouts)
	reg.MustRegister(m.PeerDiameterAnswersStalled)
	reg.MustRegister(m.RouterRoutesNotFound)
	reg.MustRegister(m.RouterHandlerErrors)

	return m
}

func newHttpClientPrometheusMetrics(reg prometheus.Registerer) *HttpClientPrometheusMetrics {
	m := &HttpClientPrometheusMetrics{
		HttpClientExchanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_client_exchanges",
				Help: "Http client exchanges",
			},
			[]string{"endpoint", "errorcode"}),
	}

	reg.MustRegister(m.HttpClientExchanges)

	return m
}

func newHttpHandlerPrometheusMetrics(reg prometheus.Registerer) *HttpHandlerPrometheusMetrics {
	m := &HttpHandlerPrometheusMetrics{
		HttpHandlerExchanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_handler_exchanges",
				Help: "Http handler exchanges",
			},
			[]string{"path", "errorcode"}),
	}

	reg.MustRegister(m.HttpHandlerExchanges)

	return m
}

func newHttpRouterPrometheusMetrics(reg prometheus.Registerer) *HttpRouterPrometheusMetrics {
	m := &HttpRouterPrometheusMetrics{
		HttpRouterExchanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_router_exchanges",
				Help: "Http router exchanges",
			},
			[]string{"path", "errorcode"}),
	}

	reg.MustRegister(m.HttpRouterExchanges)

	return m
}

func newSessionServerPrometheusMetrics(reg prometheus.Registerer) *SessionServerPrometheusMetrics {
	m := &SessionServerPrometheusMetrics{
		SessionQueries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "session_server_queries",
				Help: "Session Server queries",
			},
			[]string{"path", "indexname", "errorcode"}),

		SessionUpdates: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "session_server_updates",
				Help: "Session Server updates",
			},
			[]string{"code", "errorcode", "offending_index"}),

		SessionCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "session_server_sessions",
				Help: "Total number of sessions",
			},
			[]string{}),
	}

	reg.MustRegister(m.SessionQueries)
	reg.MustRegister(m.SessionUpdates)
	reg.MustRegister(m.SessionCount)

	return m
}

// Helper functions

// Radius
func RecordRadiusServerRequest(endpoint string, code string) {
	pm.RadiusMetrics.RadiusServerRequests.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusServerResponse(endpoint string, code string) {
	pm.RadiusMetrics.RadiusServerResponses.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusServerDrop(endpoint string, code string) {
	pm.RadiusMetrics.RadiusServerDrops.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusClientRequest(endpoint string, code string) {
	pm.RadiusMetrics.RadiusClientRequests.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusClientResponse(endpoint string, code string) {
	pm.RadiusMetrics.RadiusClientResponses.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusClientTimeout(endpoint string, code string) {
	pm.RadiusMetrics.RadiusClientTimeouts.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusClientResponseStalled(endpoint string, code string) {
	pm.RadiusMetrics.RadiusClientResponsesStalled.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

func RecordRadiusClientResponseDrop(endpoint string, code string) {
	pm.RadiusMetrics.RadiusClientResponsesDropped.With(prometheus.Labels{"endpoint": endpoint, "code": code}).Inc()
}

// Diameter

func LabelsFromDiameterMessage(peerName string, diameterMessage *DiameterMessage) prometheus.Labels {
	return prometheus.Labels{
		"peer": peerName,
		"oh":   diameterMessage.GetStringAVP("Origin-Host"),
		"or":   diameterMessage.GetStringAVP("Origin-Realm"),
		"dh":   diameterMessage.GetStringAVP("Destination-Host"),
		"dr":   diameterMessage.GetStringAVP("Destination-Realm"),
		"ap":   diameterMessage.ApplicationName,
		"cm":   diameterMessage.CommandName,
	}
}

func RecordPeerDiameterRequestReceived(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.PeerDiameterRequestsReceived.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordPeerDiameterAnswerSent(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.PeerDiameterAnswersSent.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordPeerDiameterRequestSent(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.PeerDiameterRequestsSent.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordPeerDiameterAnswerReceived(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.PeerDiameterAnswersReceived.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordPeerDiameterRequestTimeout(labels prometheus.Labels) {
	pm.DiameterMetrics.PeerDiameterRequestTimeouts.With(labels).Inc()
}

func RecordPeerDiameterAnswerStalled(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.PeerDiameterAnswersStalled.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

// Router

func RecordRouterRouteNotFound(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.RouterRoutesNotFound.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordRouterNoAvailablePeer(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.RouterPeerNotAvailable.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

func RecordRouterHandlerError(peerName string, diameterMessage *DiameterMessage) {
	pm.DiameterMetrics.RouterHandlerErrors.With(LabelsFromDiameterMessage(peerName, diameterMessage)).Inc()
}

// Http

func RecordHttpClientExchange(endpoint string, errorCode string) {
	pm.HttpClientMetrics.HttpClientExchanges.With(prometheus.Labels{"endpoint": endpoint, "errorcode": errorCode}).Inc()
}

func RecordHttpHandlerExchange(path string, errorCode string) {
	pm.HttpHandlerMetrics.HttpHandlerExchanges.With(prometheus.Labels{"path": path, "errorcode": errorCode}).Inc()
}

func RecordHttpRouterExchange(path string, errorCode string) {
	// Strip the querystring to the path
	pos := strings.IndexRune(path, '?')
	if pos >= 0 {
		path = path[:pos]
	}
	pm.HttpRouterMetrics.HttpRouterExchanges.With(prometheus.Labels{"path": path, "errorcode": errorCode}).Inc()
}

// Session Server

func RecordSessionQuery(path string, indexname string, errorCode string) {
	pm.SessionServerMetrics.SessionQueries.With(prometheus.Labels{"path": path, "indexname": indexname, "errorcode": errorCode}).Inc()
}

func RecordSessionUpdate(code string, errorCode string, offendingIndex string) {
	pm.SessionServerMetrics.SessionUpdates.With(prometheus.Labels{"code": code, "errorcode": errorCode, "offending_index": offendingIndex}).Inc()
}

func UpdateSessionCounter(nSessions int) {
	pm.SessionServerMetrics.SessionCount.With(prometheus.Labels{}).Set(float64(nSessions))
}

// Helper for testing
func GetMetricWithLabels(metricName string, labelString string) (string, error) {
	metrics, err := HttpGet("http://localhost:9109/metrics")
	if err != nil {
		return "", err
	}

	regex, err := regexp.Compile(fmt.Sprintf("%s%s ([0-9\\.]+)", metricName, labelString))
	if err != nil {
		return "", err
	}

	if match := regex.FindStringSubmatch(metrics); len(match) > 1 {
		return match[1], nil
	} else {
		return "", errors.New("metric and label not found")
	}

}
