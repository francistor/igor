package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Buffer for the channel to receive the events
const INPUT_QUEUE_SIZE = 100

// Buffer for the channel to receive the queries
const QUERY_QUEUE_SIZE = 10

// All DiameterPeerMetric keys
var DIAM_AGG_KEYS = []string{"Peer", "OH", "OR", "DH", "DR", "AP", "CM"}

// All RadiusMetric keys
var RADIUS_AGG_KEYS = []string{"Endpoint", "Code"}

// All HttpClientMetric keys
var HTTPCLIENT_AGG_KEYS = []string{"Endpoint", "ErrorCode"}

// All HttpHandlerMetric Keys
var HTTPHANDLER_AGG_KEYS = []string{"Path", "ErrorCode"}

// All HttpRouterMetric Keys
var HTTPROUTER_AGG_KEYS = []string{"Path", "ErrorCode"}

type ResetMetricsEvent struct{}

// The single instance of the metrics server.
var MS *MetricsServer

type MetricsServerConfiguration struct {
	BindAddress string
	Port        int
}

// Specification of a query to the metrics server. Metrics server will listen for this type
// of object in a channel
type Query struct {

	// Name of the metric to query
	Name string

	// List of labels to aggregate
	AggLabels []string

	// Map of label/values to filter
	Filter map[string]string

	// Channel where the response is written
	RChan chan interface{}
}

// The Metrics servers holds the metrics and runs an event loop for getting the events and updating the statistics,
// answering to queries and do graceful termination
type MetricsServer struct {

	// To wait until termination
	doneChan chan interface{}

	// To signal closure
	controlChan chan interface{}

	// Events for metrics updating are received here
	metricEventChan chan interface{}

	// Queries are received here
	queryChan chan Query

	// HttpServer
	httpMetricsServer *http.Server

	// Diameter Server
	diameterRequestsReceived PeerDiameterMetrics
	diameterAnswersSent      PeerDiameterMetrics

	// Diameter Client
	diameterRequestsSent    PeerDiameterMetrics
	diameterAnswersReceived PeerDiameterMetrics
	diameterRequestsTimeout PeerDiameterMetrics
	diameterAnswersStalled  PeerDiameterMetrics

	// RadiusServer
	radiusServerRequests  RadiusMetrics
	radiusServerResponses RadiusMetrics
	radiusServerDrops     RadiusMetrics

	// RadiusClient
	radiusClientRequests         RadiusMetrics
	radiusClientResponses        RadiusMetrics
	radiusClientTimeouts         RadiusMetrics
	radiusClientResponsesStalled RadiusMetrics
	radiusClientResponsesDrops   RadiusMetrics

	// Router
	diameterRouteNotFound   PeerDiameterMetrics
	diameterNoAvailablePeer PeerDiameterMetrics
	diameterHandlerError    PeerDiameterMetrics

	// HttpClient
	httpClientExchanges HttpClientMetrics

	// HttpHandler
	httpHandlerExchanges HttpHandlerMetrics

	// HttpRouter
	httpRouterExchanges HttpRouterMetrics

	// One PeerTable per configuration instance
	diameterPeersTables map[string]DiameterPeersTable
	radiusServersTables map[string]RadiusServersTable
}

func NewMetricsServer(bindAddress string, port int) *MetricsServer {
	server := MetricsServer{
		doneChan:        make(chan interface{}, 1),
		controlChan:     make(chan interface{}, 1),
		metricEventChan: make(chan interface{}, INPUT_QUEUE_SIZE), // Receives the events to record
		queryChan:       make(chan Query, QUERY_QUEUE_SIZE)}       // Receives the queries

	// Initialize Metrics
	server.resetMetrics()
	server.diameterPeersTables = make(map[string]DiameterPeersTable, 1)
	server.radiusServersTables = make(map[string]RadiusServersTable, 1)

	// Start metrics server
	go server.httpLoop(bindAddress, port)

	// Start metrics processing loop
	go server.metricServerLoop()

	return &server
}

// To be called in the main function
func initMetricsServer(cm *ConfigurationManager) {

	var metricsConfig = NewConfigObject[MetricsServerConfiguration]("metrics.json")
	if err := metricsConfig.Update(cm); err != nil {
		panic("could not apply metrics configuration: " + err.Error())
	}

	// Make the metrics server globally available
	var config = metricsConfig.Get()
	MS = NewMetricsServer(config.BindAddress, config.Port)
}

// Shuts down the http server and the event loop
// If ever done, make sure that the whole proces is terminating or that another
// configuration instance intizialization will take place, because MetricsServer
// initialization is done there
func (ms *MetricsServer) Close() {
	close(ms.controlChan)
	<-ms.doneChan

	// The other channels are not closed
}

////////////////////////////////////////////////////////////
// Diameter Metrics
////////////////////////////////////////////////////////////

// Returns a set of metrics in which only the properties specified in labels are not zeroed
// and the values are aggregated over the rest of labels
func GetAggPeerDiameterMetrics(peerDiameterMetrics PeerDiameterMetrics, aggLabels []string) PeerDiameterMetrics {
	outMetrics := make(PeerDiameterMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range peerDiameterMetrics {
		// metricKey will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := PeerDiameterMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "Peer":
				mk.Peer = metricKey.Peer
			case "OH":
				mk.OH = metricKey.OH
			case "OR":
				mk.OR = metricKey.OR
			case "DH":
				mk.DH = metricKey.DH
			case "DR":
				mk.DR = metricKey.DR
			case "AP":
				mk.AP = metricKey.AP
			case "CM":
				mk.CM = metricKey.CM
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

// Returns only the items in the metrics whose values correspond to the filter, which specifies
// values for certain labels
func GetFilteredPeerDiameterMetrics(peerDiameterMetrics PeerDiameterMetrics, filter map[string]string) PeerDiameterMetrics {

	// If no filter specified, do nothing
	if filter == nil {
		return peerDiameterMetrics
	}

	// We'll put the output here
	outMetrics := make(PeerDiameterMetrics)

	for metricKey := range peerDiameterMetrics {

		// Check all the items in the filter. If mismatch, get out of the outer loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "Peer":
				if metricKey.Peer != filter["Peer"] {
					match = false
					break outer
				}
			case "OH":
				if metricKey.OH != filter["OH"] {
					match = false
					break outer
				}
			case "OR":
				if metricKey.OR != filter["OR"] {
					match = false
					break outer
				}
			case "DH":
				if metricKey.DH != filter["DH"] {
					match = false
					break outer
				}
			case "DR":
				if metricKey.DR != filter["DR"] {
					match = false
					break outer
				}
			case "AP":
				if metricKey.AP != filter["AP"] {
					match = false
					break outer
				}
			case "CM":
				if metricKey.CM != filter["CM"] {
					match = false
					break outer
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = peerDiameterMetrics[metricKey]
		}
	}

	return outMetrics
}

// Gets filtered and aggregated metrics
func GetPeerDiameterMetrics(peerDiameterMetrics PeerDiameterMetrics, filter map[string]string, aggLabels []string) PeerDiameterMetrics {
	return GetAggPeerDiameterMetrics(GetFilteredPeerDiameterMetrics(peerDiameterMetrics, filter), aggLabels)
}

////////////////////////////////////////////////////////////
// Radius Metrics
////////////////////////////////////////////////////////////

func GetAggRadiusMetrics(radiusMetrics RadiusMetrics, aggLabels []string) RadiusMetrics {
	outMetrics := make(RadiusMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range radiusMetrics {
		// metricKey will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := RadiusMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "Code":
				mk.Code = metricKey.Code
			case "Endpoint":
				mk.Endpoint = metricKey.Endpoint
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

func GetFilteredRadiusMetrics(radiusMetrics RadiusMetrics, filter map[string]string) RadiusMetrics {

	// If no filter specified, do nothing
	if filter == nil {
		return radiusMetrics
	}

	// We'll put the output here
	outMetrics := make(RadiusMetrics)

	for metricKey := range radiusMetrics {

		// Check all the items in the filter. If mismatch, get out of the loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "Code":
				if metricKey.Code != filter["Code"] {
					match = false
					break outer
				}
			case "Endpoint":
				if metricKey.Endpoint != filter["Endpoint"] {
					match = false
					break outer
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = radiusMetrics[metricKey]
		}
	}

	return outMetrics
}

func GetRadiusMetrics(radiusMetrics RadiusMetrics, filter map[string]string, aggLabels []string) RadiusMetrics {
	return GetAggRadiusMetrics(GetFilteredRadiusMetrics(radiusMetrics, filter), aggLabels)
}

////////////////////////////////////////////////////////////
// Http Client Metrics
////////////////////////////////////////////////////////////

func GetAggHttpClientMetrics(httpClientMetrics HttpClientMetrics, aggLabels []string) HttpClientMetrics {
	outMetrics := make(HttpClientMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range httpClientMetrics {
		// mk will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := HttpClientMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "Endpoint":
				mk.Endpoint = metricKey.Endpoint
			case "ErrorCode":
				mk.ErrorCode = metricKey.ErrorCode
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

func GetFilteredHttpClientMetrics(httpClientMetrics HttpClientMetrics, filter map[string]string) HttpClientMetrics {

	// If no filter specified, do nothing
	if filter == nil {
		return httpClientMetrics
	}

	// We'll put the output here
	outMetrics := make(HttpClientMetrics)

	for metricKey := range httpClientMetrics {

		// Check all the items in the filter. If mismatch, get out of the loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "Endpoint":
				if metricKey.Endpoint != filter["Endpoint"] {
					match = false
					break outer
				}
			case "ErrorCode":
				if metricKey.ErrorCode != filter["ErrorCode"] {
					match = false
					break outer
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = httpClientMetrics[metricKey]
		}
	}

	return outMetrics
}

func GetHttpClientMetrics(httpClientMetrics HttpClientMetrics, filter map[string]string, aggLabels []string) HttpClientMetrics {
	return GetAggHttpClientMetrics(GetFilteredHttpClientMetrics(httpClientMetrics, filter), aggLabels)
}

////////////////////////////////////////////////////////////
// Http Handler Metrics
////////////////////////////////////////////////////////////

func GetAggHttpHandlerMetrics(httpHandlerMetrics HttpHandlerMetrics, aggLabels []string) HttpHandlerMetrics {
	outMetrics := make(HttpHandlerMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range httpHandlerMetrics {
		// metricKey will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := HttpHandlerMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "ErrorCode":
				mk.ErrorCode = metricKey.ErrorCode
			case "Path":
				mk.Path = metricKey.Path
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

func GetFilteredHttpHandlerMetrics(httpHandlerMetrics HttpHandlerMetrics, filter map[string]string) HttpHandlerMetrics {

	// If no filter specified, do nothing
	if filter == nil {
		return httpHandlerMetrics
	}

	// We'll put the output here
	outMetrics := make(HttpHandlerMetrics)

	for metricKey := range httpHandlerMetrics {

		// Check all the items in the filter. If mismatch, get out of the loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "ErrorCode":
				if metricKey.ErrorCode != filter["ErrorCode"] {
					match = false
					break outer
				}
			case "Path":
				if metricKey.Path != filter["Path"] {
					match = false
					break outer
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = httpHandlerMetrics[metricKey]
		}
	}

	return outMetrics
}

func GetHttpHandlerMetrics(httpHandlerMetrics HttpHandlerMetrics, filter map[string]string, aggLabels []string) HttpHandlerMetrics {
	return GetAggHttpHandlerMetrics(GetFilteredHttpHandlerMetrics(httpHandlerMetrics, filter), aggLabels)
}

////////////////////////////////////////////////////////////
// Http Router Metrics
////////////////////////////////////////////////////////////

func GetAggHttpRouterMetrics(httpRouterMetrics HttpRouterMetrics, aggLabels []string) HttpRouterMetrics {
	outMetrics := make(HttpRouterMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range httpRouterMetrics {
		// metricKey will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := HttpRouterMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "ErrorCode":
				mk.ErrorCode = metricKey.ErrorCode
			case "Path":
				mk.Path = metricKey.Path
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

func GetFilteredHttpRouterMetrics(httpRouterMetrics HttpRouterMetrics, filter map[string]string) HttpRouterMetrics {

	// If no filter specified, do nothing
	if filter == nil {
		return httpRouterMetrics
	}

	// We'll put the output here
	outMetrics := make(HttpRouterMetrics)

	for metricKey := range httpRouterMetrics {

		// Check all the items in the filter. If mismatch, get out of the loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "ErrorCode":
				if metricKey.ErrorCode != filter["ErrorCode"] {
					match = false
					break outer
				}
			case "Path":
				if metricKey.Path != filter["Path"] {
					match = false
					break outer
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = httpRouterMetrics[metricKey]
		}
	}

	return outMetrics
}

func GetHttpRouterMetrics(httpRouterMetrics HttpRouterMetrics, filter map[string]string, aggLabels []string) HttpRouterMetrics {
	return GetAggHttpRouterMetrics(GetFilteredHttpRouterMetrics(httpRouterMetrics, filter), aggLabels)
}

//////////////////////////////////////////////////////////////////////////////////

// Empties all the counters
func (ms *MetricsServer) resetMetrics() {
	ms.diameterRequestsReceived = make(PeerDiameterMetrics)
	ms.diameterAnswersSent = make(PeerDiameterMetrics)

	ms.diameterRequestsSent = make(PeerDiameterMetrics)
	ms.diameterAnswersReceived = make(PeerDiameterMetrics)
	ms.diameterRequestsTimeout = make(PeerDiameterMetrics)
	ms.diameterAnswersStalled = make(PeerDiameterMetrics)

	ms.diameterRouteNotFound = make(PeerDiameterMetrics)
	ms.diameterNoAvailablePeer = make(PeerDiameterMetrics)
	ms.diameterHandlerError = make(PeerDiameterMetrics)

	ms.radiusServerRequests = make(RadiusMetrics)
	ms.radiusServerResponses = make(RadiusMetrics)
	ms.radiusServerDrops = make(RadiusMetrics)

	ms.radiusClientRequests = make(RadiusMetrics)
	ms.radiusClientResponses = make(RadiusMetrics)
	ms.radiusClientTimeouts = make(RadiusMetrics)
	ms.radiusClientResponsesStalled = make(RadiusMetrics)
	ms.radiusClientResponsesDrops = make(RadiusMetrics)

	ms.httpClientExchanges = make(HttpClientMetrics)

	ms.httpHandlerExchanges = make(HttpHandlerMetrics)

	ms.httpRouterExchanges = make(HttpRouterMetrics)
}

// Wrapper to reset Diameter Metrics
func (ms *MetricsServer) ResetMetrics() {
	ms.metricEventChan <- ResetMetricsEvent{}
}

// Wrapper to get Diameter Metrics
func (ms *MetricsServer) DiameterQuery(name string, filter map[string]string, aggLabels []string) PeerDiameterMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.queryChan <- query
	v, ok := (<-query.RChan).(PeerDiameterMetrics)
	if ok {
		return v
	} else {
		return PeerDiameterMetrics{}
	}
}

// Wrapper to get Radius Metrics
func (ms *MetricsServer) RadiusQuery(name string, filter map[string]string, aggLabels []string) RadiusMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.queryChan <- query
	v, ok := (<-query.RChan).(RadiusMetrics)
	if ok {
		return v
	} else {
		return RadiusMetrics{}
	}
}

// Wrapper to get HttpClient metrics
func (ms *MetricsServer) HttpClientQuery(name string, filter map[string]string, aggLabels []string) HttpClientMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.queryChan <- query
	v, ok := (<-query.RChan).(HttpClientMetrics)
	if ok {
		return v
	} else {
		return HttpClientMetrics{}
	}
}

// Wrapper to get HttpHandler metrics
func (ms *MetricsServer) HttpHandlerQuery(name string, filter map[string]string, aggLabels []string) HttpHandlerMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.queryChan <- query
	v, ok := (<-query.RChan).(HttpHandlerMetrics)
	if ok {
		return v
	} else {
		return HttpHandlerMetrics{}
	}
}

// Wrapper to get HttpRouter metrics
func (ms *MetricsServer) HttpRouterQuery(name string, filter map[string]string, aggLabels []string) HttpRouterMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.queryChan <- query
	v, ok := (<-query.RChan).(HttpRouterMetrics)
	if ok {
		return v
	} else {
		return HttpRouterMetrics{}
	}
}

// Wrapper to get PeersTable
func (ms *MetricsServer) PeersTableQuery() map[string]DiameterPeersTable {
	query := Query{Name: "DiameterPeersTables", RChan: make(chan interface{})}
	ms.queryChan <- query
	return (<-query.RChan).(map[string]DiameterPeersTable)
}

// Wrapper to get RadiusServersTable
func (ms *MetricsServer) RadiusServersTableQuery() map[string]RadiusServersTable {
	query := Query{Name: "RadiusServersTables", RChan: make(chan interface{})}
	ms.queryChan <- query
	return (<-query.RChan).(map[string]RadiusServersTable)
}

// Loop for Prometheus metrics server
func (ms *MetricsServer) httpLoop(bindAddress string, port int) {

	mux := new(http.ServeMux)
	mux.HandleFunc("/metrics", ms.getPrometheusMetricsHandler())
	mux.HandleFunc("/diameterPeers", ms.getDiameterPeersHandler())
	mux.HandleFunc("/radiusServers", ms.getRadiusServersHandler())
	// Same for all below
	mux.HandleFunc("/diameterMetrics/", ms.getMetricsHandler())
	mux.HandleFunc("/radiusMetrics/", ms.getMetricsHandler())
	mux.HandleFunc("/httpClientMetrics/", ms.getMetricsHandler())
	mux.HandleFunc("/httpRouterMetrics/", ms.getMetricsHandler())
	mux.HandleFunc("/httpHandlerMetrics/", ms.getMetricsHandler())

	bindAddrPort := fmt.Sprintf("%s:%d", bindAddress, port)
	GetLogger().Infof("prometheus metrics server listening in %s", bindAddrPort)

	ms.httpMetricsServer = &http.Server{
		Addr:              bindAddrPort,
		Handler:           mux,
		IdleTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Prometheus uses plain old http
	err := ms.httpMetricsServer.ListenAndServe()

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting metrics handler: " + err.Error())
	}

	// Will get here only when a shutdown is invoked
	close(ms.doneChan)
}

// Main loop for getting metrics and serving queries
func (ms *MetricsServer) metricServerLoop() {

	for {
		select {

		case <-ms.controlChan:
			// Shutdown server
			ms.httpMetricsServer.Shutdown(context.Background())
			return

		case query := <-ms.queryChan:

			switch query.Name {
			case "DiameterRequestsReceived":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterRequestsReceived, query.Filter, query.AggLabels)
			case "DiameterAnswersSent":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterAnswersSent, query.Filter, query.AggLabels)

			case "DiameterRequestsSent":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterRequestsSent, query.Filter, query.AggLabels)
			case "DiameterAnswersReceived":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterAnswersReceived, query.Filter, query.AggLabels)
			case "DiameterRequestsTimeout":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterRequestsTimeout, query.Filter, query.AggLabels)
			case "DiameterAnswersStalled":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterAnswersSent, query.Filter, query.AggLabels)

			case "DiameterRouteNotFound":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterRouteNotFound, query.Filter, query.AggLabels)
			case "DiameterNoAvailablePeer":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterNoAvailablePeer, query.Filter, query.AggLabels)
			case "DiameterHandlerError":
				query.RChan <- GetPeerDiameterMetrics(ms.diameterHandlerError, query.Filter, query.AggLabels)

			case "RadiusServerRequests":
				query.RChan <- GetRadiusMetrics(ms.radiusServerRequests, query.Filter, query.AggLabels)
			case "RadiusServerResponses":
				query.RChan <- GetRadiusMetrics(ms.radiusServerResponses, query.Filter, query.AggLabels)
			case "RadiusServerDrops":
				query.RChan <- GetRadiusMetrics(ms.radiusServerDrops, query.Filter, query.AggLabels)

			case "RadiusClientRequests":
				query.RChan <- GetRadiusMetrics(ms.radiusClientRequests, query.Filter, query.AggLabels)
			case "RadiusClientResponses":
				query.RChan <- GetRadiusMetrics(ms.radiusClientResponses, query.Filter, query.AggLabels)
			case "RadiusClientTimeouts":
				query.RChan <- GetRadiusMetrics(ms.radiusClientTimeouts, query.Filter, query.AggLabels)
			case "RadiusClientResponsesStalled":
				query.RChan <- GetRadiusMetrics(ms.radiusClientResponsesStalled, query.Filter, query.AggLabels)
			case "RadiusClientResponsesDrops":
				query.RChan <- GetRadiusMetrics(ms.radiusClientResponsesDrops, query.Filter, query.AggLabels)

			case "HttpClientExchanges":
				query.RChan <- GetHttpClientMetrics(ms.httpClientExchanges, query.Filter, query.AggLabels)

			case "HttpHandlerExchanges":
				query.RChan <- GetHttpHandlerMetrics(ms.httpHandlerExchanges, query.Filter, query.AggLabels)

			case "HttpRouterExchanges":
				query.RChan <- GetHttpRouterMetrics(ms.httpRouterExchanges, query.Filter, query.AggLabels)

			case "DiameterPeersTables":
				query.RChan <- ms.diameterPeersTables

			case "RadiusServersTables":
				query.RChan <- ms.radiusServersTables
			}

			close(query.RChan)

		case event, ok := <-ms.metricEventChan:

			if !ok {
				break
			}

			switch e := event.(type) {

			case ResetMetricsEvent:
				ms.resetMetrics()

			// Diameter Events
			case PeerDiameterRequestReceivedEvent:
				if curr, ok := ms.diameterRequestsReceived[e.Key]; !ok {
					ms.diameterRequestsReceived[e.Key] = 1
				} else {
					ms.diameterRequestsReceived[e.Key] = curr + 1
				}
			case PeerDiameterAnswerSentEvent:
				if curr, ok := ms.diameterAnswersSent[e.Key]; !ok {
					ms.diameterAnswersSent[e.Key] = 1
				} else {
					ms.diameterAnswersSent[e.Key] = curr + 1
				}

			case PeerDiameterRequestSentEvent:
				if curr, ok := ms.diameterRequestsSent[e.Key]; !ok {
					ms.diameterRequestsSent[e.Key] = 1
				} else {
					ms.diameterRequestsSent[e.Key] = curr + 1
				}

			case PeerDiameterAnswerReceivedEvent:
				if curr, ok := ms.diameterAnswersReceived[e.Key]; !ok {
					ms.diameterAnswersReceived[e.Key] = 1
				} else {
					ms.diameterAnswersReceived[e.Key] = curr + 1
				}

			case PeerDiameterRequestTimeoutEvent:
				if curr, ok := ms.diameterRequestsTimeout[e.Key]; !ok {
					ms.diameterRequestsTimeout[e.Key] = 1
				} else {
					ms.diameterRequestsTimeout[e.Key] = curr + 1
				}

			// Radius Events
			case PeerDiameterAnswerStalledEvent:
				if curr, ok := ms.diameterAnswersStalled[e.Key]; !ok {
					ms.diameterAnswersStalled[e.Key] = 1
				} else {
					ms.diameterAnswersStalled[e.Key] = curr + 1
				}

			case RadiusServerRequestEvent:
				if curr, ok := ms.radiusServerRequests[e.Key]; !ok {
					ms.radiusServerRequests[e.Key] = 1
				} else {
					ms.radiusServerRequests[e.Key] = curr + 1
				}

			case RadiusServerResponseEvent:
				if curr, ok := ms.radiusServerResponses[e.Key]; !ok {
					ms.radiusServerResponses[e.Key] = 1
				} else {
					ms.radiusServerResponses[e.Key] = curr + 1
				}

			case RadiusServerDropEvent:
				if curr, ok := ms.radiusServerDrops[e.Key]; !ok {
					ms.radiusServerDrops[e.Key] = 1
				} else {
					ms.radiusServerDrops[e.Key] = curr + 1
				}

			case RadiusClientRequestEvent:
				if curr, ok := ms.radiusClientRequests[e.Key]; !ok {
					ms.radiusClientRequests[e.Key] = 1
				} else {
					ms.radiusClientRequests[e.Key] = curr + 1
				}

			case RadiusClientResponseEvent:
				if curr, ok := ms.radiusClientResponses[e.Key]; !ok {
					ms.radiusClientResponses[e.Key] = 1
				} else {
					ms.radiusClientResponses[e.Key] = curr + 1
				}

			case RadiusClientTimeoutEvent:
				if curr, ok := ms.radiusClientTimeouts[e.Key]; !ok {
					ms.radiusClientTimeouts[e.Key] = 1
				} else {
					ms.radiusClientTimeouts[e.Key] = curr + 1
				}

			case RadiusClientResponseStalledEvent:
				if curr, ok := ms.radiusClientResponsesStalled[e.Key]; !ok {
					ms.radiusClientResponsesStalled[e.Key] = 1
				} else {
					ms.radiusClientResponsesStalled[e.Key] = curr + 1
				}

			case RadiusClientResponseDropEvent:
				if curr, ok := ms.radiusClientResponsesDrops[e.Key]; !ok {
					ms.radiusClientResponsesDrops[e.Key] = 1
				} else {
					ms.radiusClientResponsesDrops[e.Key] = curr + 1
				}

			// Router Events

			case RouterRouteNotFoundEvent:
				if curr, ok := ms.diameterRouteNotFound[e.Key]; !ok {
					ms.diameterRouteNotFound[e.Key] = 1
				} else {
					ms.diameterRouteNotFound[e.Key] = curr + 1
				}
			case RouterNoAvailablePeerEvent:
				if curr, ok := ms.diameterNoAvailablePeer[e.Key]; !ok {
					ms.diameterNoAvailablePeer[e.Key] = 1
				} else {
					ms.diameterNoAvailablePeer[e.Key] = curr + 1
				}
			case RouterHandlerError:
				if curr, ok := ms.diameterHandlerError[e.Key]; !ok {
					ms.diameterHandlerError[e.Key] = 1
				} else {
					ms.diameterHandlerError[e.Key] = curr + 1
				}

			// HttpClient Events
			case HttpClientExchangeEvent:
				if curr, ok := ms.httpClientExchanges[e.Key]; !ok {
					ms.httpClientExchanges[e.Key] = 1
				} else {
					ms.httpClientExchanges[e.Key] = curr + 1
				}

			// HttpHandler Events
			case HttpHandlerExchangeEvent:
				if curr, ok := ms.httpHandlerExchanges[e.Key]; !ok {
					ms.httpHandlerExchanges[e.Key] = 1
				} else {
					ms.httpHandlerExchanges[e.Key] = curr + 1
				}

			// HttpHandler Events
			case HttpRouterExchangeEvent:
				if curr, ok := ms.httpRouterExchanges[e.Key]; !ok {
					ms.httpRouterExchanges[e.Key] = 1
				} else {
					ms.httpRouterExchanges[e.Key] = curr + 1
				}

			// PeersTable
			case DiameterPeersTableUpdatedEvent:
				ms.diameterPeersTables[e.InstanceName] = e.Table

			// RadiusTable
			case RadiusServersTableUpdatedEvent:
				ms.radiusServersTables[e.InstanceName] = e.Table
			}
		}
	}
}

// /////////////////////////////////////
// Handlers for http metrics server

func (ms *MetricsServer) getDiameterPeersHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		diameterPeerTables := ms.PeersTableQuery()
		jAnswer, err := json.Marshal(diameterPeerTables)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			GetLogger().Errorf("could not marshal PeersTables due to: %s", err.Error())
			return
		}
		writer.Header().Add("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write(jAnswer)
	}
}

func (ms *MetricsServer) getRadiusServersHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		radiusServerTables := ms.RadiusServersTableQuery()
		jAnswer, err := json.Marshal(radiusServerTables)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			GetLogger().Errorf("could not marshal radiusServerTables due to: %s", err.Error())
			return
		}
		writer.Header().Add("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write(jAnswer)
	}
}

// For endpoints of the form
// /<metricType>/<metricname>?agg=<comma-separated-labels>&filterkeya=filtervalue1&filterkey2=filtervalue2
func (ms *MetricsServer) getMetricsHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		// Get parameters
		filter := make(map[string]string)
		for q, v := range request.URL.Query() {
			if q != "agg" {
				filter[q] = v[0]
			}
		}
		agg := strings.Split(request.URL.Query().Get("agg"), ",")
		pathElements := strings.Split(request.URL.Path, "/")
		// After splitting, there is a "" in the beginning
		if len(pathElements) != 3 || len(pathElements[2]) < 2 {
			writer.WriteHeader(http.StatusNotFound)
			GetLogger().Errorf("bad path in metrics query: %s" + request.URL.Path)
			return
		}
		queryName := pathElements[2]
		// Capitalize first letter
		queryName = strings.ToUpper(queryName[0:1]) + queryName[1:]

		var jAnswer []byte
		var err error
		switch pathElements[1] {
		case "diameterMetrics":
			jAnswer, err = json.Marshal(ms.DiameterQuery(queryName, filter, agg))
		case "radiusMetrics":
			jAnswer, err = json.Marshal(ms.RadiusQuery(queryName, filter, agg))
		case "httpClientMetrics":
			jAnswer, err = json.Marshal(ms.HttpClientQuery(queryName, filter, agg))
		case "httpRouterMetrics":
			jAnswer, err = json.Marshal(ms.HttpClientQuery(queryName, filter, agg))
		case "httpHandlerMetrics":
			jAnswer, err = json.Marshal(ms.HttpClientQuery(queryName, filter, agg))
		}

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			GetLogger().Errorf("could not marshal metric due to: %s", err.Error())
			return
		}
		writer.Header().Add("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write(jAnswer)
	}
}

func (ms *MetricsServer) getPrometheusMetricsHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusOK)

		var builder strings.Builder

		// Cannot access directly the ms variables, but go through the event loop
		// Diameter Server
		builder.WriteString(ms.DiameterQuery("DiameterRequestsReceived", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_requests_received", "number of diameter requests received"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterAnswersSent", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_answers_sent", "number of diameter answers sent"))
		builder.WriteString("\n")
		// Diameter client
		builder.WriteString(ms.DiameterQuery("DiameterRequestsSent", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_requests_sent", "number of diameter requests sent"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterAnswersReceived", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_answers_received", "number of diameter answers received"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterRequestsTimeout", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_requests_timeout", "number of diameter requests timed out"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterAnswersStalled", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_answers_stalled", "number of diameter answers without corresponding request, possibly due to previous timeout"))
		builder.WriteString("\n")
		// Radius server
		builder.WriteString(ms.RadiusQuery("RadiusServerRequests", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_server_requests", "number of radius server requests received"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusServerResponses", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_server_responses", "number of radius server responses sent"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusServerDrops", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_server_drops", "number of radius server requests not answered"))
		builder.WriteString("\n")
		// Radius client
		builder.WriteString(ms.RadiusQuery("RadiusClientRequests", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_client_requests", "number of radius client requests sent"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusClientResponses", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_client_responses", "number of radius client responses received"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusClientTimeouts", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_client_timeouts", "number of radius client timeouts"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusClientResponsesStalled", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_client_responses_stalled", "number of radius client responses without corresponding request, possibly due to previous timeout"))
		builder.WriteString("\n")
		builder.WriteString(ms.RadiusQuery("RadiusClientResponsesDrops", nil, RADIUS_AGG_KEYS).genPrometheusMetric("radius_client_responses_drops", "number of radius client responses dropped"))
		builder.WriteString("\n")
		// Router
		builder.WriteString(ms.DiameterQuery("DiameterRouteNotFound", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_route_not_found", "diameter messages dropped due to route not found"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterNoAvailablePeer", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_no_available_peer", "diameter messages dropped due to no peer available"))
		builder.WriteString("\n")
		builder.WriteString(ms.DiameterQuery("DiameterHandlerError", nil, DIAM_AGG_KEYS).genPrometheusMetric("diameter_handler_error", "diameter handler errors"))
		builder.WriteString("\n")
		// HttpClient
		builder.WriteString(ms.HttpClientQuery("HttpClientExchanges", nil, HTTPCLIENT_AGG_KEYS).genPrometheusMetric("http_client_exchanges", "requests sent to http handler"))
		builder.WriteString("\n")
		// HttpHandler
		builder.WriteString(ms.HttpHandlerQuery("HttpHandlerExchanges", nil, HTTPHANDLER_AGG_KEYS).genPrometheusMetric("http_handler_exchanges", "requests received in http handler"))
		builder.WriteString("\n")
		// HttpRouter
		builder.WriteString(ms.HttpRouterQuery("HttpRouterExchanges", nil, HTTPROUTER_AGG_KEYS).genPrometheusMetric("http_router_exchanges", "requests received in http router"))
		builder.WriteString("\n")

		// Write response
		writer.Write([]byte(builder.String()))
	}

}
