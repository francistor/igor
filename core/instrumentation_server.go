package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Instrumentation of Diameter Peers table
type DiameterPeersTableEntry struct {
	DiameterHost     string
	IPAddress        string
	ConnectionPolicy string
	IsEngaged        bool
	LastStatusChange time.Time
	LastError        error
}

type DiameterPeersTable []DiameterPeersTableEntry

type DiameterPeersTableUpdatedEvent struct {
	InstanceName string
	Table        DiameterPeersTable
}

func PushDiameterPeersStatus(instanceName string, table DiameterPeersTable) {
	MS.metricEventChan <- DiameterPeersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}

// Instrumentation of Radius servers table
type RadiusServerTableEntry struct {
	ServerName       string
	IsAvailable      bool
	UnavailableUntil time.Time
}

type RadiusServersTable []RadiusServerTableEntry

type RadiusServersTableUpdatedEvent struct {
	InstanceName string
	Table        RadiusServersTable
}

func PushRadiusServersTable(instanceName string, table RadiusServersTable) {
	MS.metricEventChan <- RadiusServersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}

// Buffer for the channel to receive the events
const INPUT_QUEUE_SIZE = 10

// Buffer for the channel to receive the queries
const QUERY_QUEUE_SIZE = 10

// The single instance of the metrics server.
var MS *InstrumentationServer

type InstrumentationServerConfiguration struct {
	BindAddress string
	Port        int
}

// Specification of a query to the metrics server. Metrics server will listen for this type
// of object in a channel
type Query struct {

	// Name of the metric to query
	Name string

	// Channel where the response is written
	RChan chan interface{}
}

// The Metrics servers holds the metrics and runs an event loop for getting the events and updating the statistics,
// answering to queries and do graceful termination
type InstrumentationServer struct {

	// To wait until termination
	doneChan chan interface{}

	// To signal closure
	controlChan chan interface{}

	// Events for metrics updating are received here
	metricEventChan chan interface{}

	// Queries are received here
	queryChan chan Query

	// Prometheus registry
	prometheusRegistry *prometheus.Registry

	// HttpServer
	httpMetricsServer *http.Server

	// One Table per configuration instance
	diameterPeersTables map[string]DiameterPeersTable
	radiusServersTables map[string]RadiusServersTable
}

func NewMetricsServer(bindAddress string, port int) *InstrumentationServer {
	server := InstrumentationServer{
		doneChan:           make(chan interface{}, 1),
		controlChan:        make(chan interface{}, 1),
		metricEventChan:    make(chan interface{}, INPUT_QUEUE_SIZE), // Receives the events to record
		queryChan:          make(chan Query, QUERY_QUEUE_SIZE),       // Receives the queries
		prometheusRegistry: prometheus.NewRegistry(),
	}

	// Initialize Metrics
	server.diameterPeersTables = make(map[string]DiameterPeersTable, 1)
	server.radiusServersTables = make(map[string]RadiusServersTable, 1)

	pm.RadiusMetrics = newRadiusPrometheusMetrics(server.prometheusRegistry)
	pm.DiameterMetrics = newDiameterPrometheusMetrics(server.prometheusRegistry)
	pm.HttpClientMetrics = newHttpClientPrometheusMetrics(server.prometheusRegistry)
	pm.HttpHandlerMetrics = newHttpHandlerPrometheusMetrics(server.prometheusRegistry)
	pm.HttpRouterMetrics = newHttpRouterPrometheusMetrics(server.prometheusRegistry)
	pm.SessionServerMetrics = newSessionServerPrometheusMetrics(server.prometheusRegistry)

	// Start metrics server
	go server.httpLoop(bindAddress, port)

	// Start metrics processing loop
	go server.metricServerLoop()

	return &server
}

// To be called in the main function
func initInstrumentationServer(cm *ConfigurationManager) {

	var metricsConfig = NewConfigObject[InstrumentationServerConfiguration]("metrics.json")
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
func (is *InstrumentationServer) Close() {
	close(is.controlChan)
	<-is.doneChan

	// The other channels are not closed
}

// Sets all counters to zero
func (is *InstrumentationServer) ResetMetrics() {
	pm.RadiusMetrics.reset()
	pm.DiameterMetrics.reset()
	pm.HttpClientMetrics.reset()
	pm.HttpHandlerMetrics.reset()
	pm.HttpRouterMetrics.reset()
	pm.SessionServerMetrics.reset()
}

//////////////////////////////////////////////////////////////////////////////////

// Wrapper to get PeersTable
func (is *InstrumentationServer) PeersTableQuery() map[string]DiameterPeersTable {
	query := Query{Name: "DiameterPeersTables", RChan: make(chan interface{})}
	is.queryChan <- query
	return (<-query.RChan).(map[string]DiameterPeersTable)
}

// Wrapper to get RadiusServersTable
func (is *InstrumentationServer) RadiusServersTableQuery() map[string]RadiusServersTable {
	query := Query{Name: "RadiusServersTables", RChan: make(chan interface{})}
	is.queryChan <- query
	return (<-query.RChan).(map[string]RadiusServersTable)
}

// Loop for Prometheus metrics server
func (is *InstrumentationServer) httpLoop(bindAddress string, port int) {

	mux := new(http.ServeMux)
	mux.Handle("/go_metrics", promhttp.Handler())
	mux.Handle("/metrics", promhttp.HandlerFor(is.prometheusRegistry, promhttp.HandlerOpts{Registry: is.prometheusRegistry}))
	mux.HandleFunc("/diameterPeers", is.getDiameterPeersHandler())
	mux.HandleFunc("/radiusServers", is.getRadiusServersHandler())

	bindAddrPort := fmt.Sprintf("%s:%d", bindAddress, port)
	GetLogger().Infof("instrumentation server listening in %s", bindAddrPort)

	is.httpMetricsServer = &http.Server{
		Addr:              bindAddrPort,
		Handler:           mux,
		IdleTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Prometheus uses plain old http
	err := is.httpMetricsServer.ListenAndServe()

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting instrumentation handler: " + err.Error())
	}

	// Will get here only when a shutdown is invoked
	close(is.doneChan)
}

// Main loop for getting metrics and serving queries
func (is *InstrumentationServer) metricServerLoop() {

	for {
		select {

		case <-is.controlChan:
			// Shutdown server
			is.httpMetricsServer.Shutdown(context.Background())
			return

		case query := <-is.queryChan:

			switch query.Name {

			case "DiameterPeersTables":
				query.RChan <- is.diameterPeersTables

			case "RadiusServersTables":
				query.RChan <- is.radiusServersTables
			}

			close(query.RChan)

		case event, ok := <-is.metricEventChan:

			if !ok {
				break
			}

			switch e := event.(type) {

			// PeersTable
			case DiameterPeersTableUpdatedEvent:
				is.diameterPeersTables[e.InstanceName] = e.Table

			// RadiusTable
			case RadiusServersTableUpdatedEvent:
				is.radiusServersTables[e.InstanceName] = e.Table
			}
		}
	}
}

func (is *InstrumentationServer) getDiameterPeersHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		diameterPeerTables := is.PeersTableQuery()
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

func (is *InstrumentationServer) getRadiusServersHandler() func(w http.ResponseWriter, req *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		radiusServerTables := is.RadiusServersTableQuery()
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
