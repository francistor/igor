package sessionserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/francistor/igor/constants"
	"github.com/francistor/igor/core"
	"github.com/francistor/igor/radiusserver"
)

// Control messages to the Event Loop.
// Implementation of the Actor model.

// Signal to initiate the termination process
type SessionServerSetDownCommand struct {
}

// Signal to terminate the event loop
type SessionServerCloseCommand struct {
}

// Message to update the SessionsStore with a new or updated session
type SessionStoreRequest struct {
	RChan  chan error
	Packet *core.RadiusPacket
}

// Message to request a query by index
type SessionQueryRequest struct {
	RChan      chan interface{} // Channel where to send the response
	IndexName  string           // Index being queried
	IndexValue string           // Value to be queried
	ActiveOnly bool             // Whether to filter the stopped sessions in the answer
}

// Represents the response to the query by index
type SessionQueryResponse struct {
	Items [][]core.RadiusAVP
}

// Specific type of error, to distinguish from normal operational errors
type IndexConstraintError struct {
	offendingIndex string
}

// Implementation of the Error interface for IndexConstraintError
func (e IndexConstraintError) Error() string {
	return e.offendingIndex
}

// Statuses of the SessionServer
const (
	StatusOperational = 0
	StatusTerminated  = 1
)

// Implements a session store that
// - Is updated via Radius requests
// - Can be queried by http
// - Communicates with other RadiusSessionServers
//   - Sending copies of the received radius requests
//   - Sending, when requested, a full dump of the store
type RadiusSessionServer struct {
	RadiusSessionStore

	// Status. Can be one of the constants above
	status int

	// Holds the radius server for incoming session storage requests
	radiusServer *radiusserver.RadiusServer

	// Holds the httpserver for incoming queries
	httpServer *http.Server

	// For signaling finalization of the http server
	httpDoneChannel chan struct{}

	// To signal that the event loop is terminated. No more events will be tried to process
	eventLoopDoneChannel chan struct{}

	// For sending session updates to the event loop
	updateChannel chan *SessionStoreRequest

	// For sending queries to the event loop
	queryChannel chan *SessionQueryRequest

	// For sending control commands to the event loop (e.g. termination)
	controlChannel chan interface{}

	// Full configuration
	config core.RadiusSessionServerConfig
}

// Creates a new instance of the RadiusSessionServer
func NewRadiusSessionServer(instanceName string) *RadiusSessionServer {

	// Get the configuration
	ssConf := core.GetRadiusSessionServerConfigInstance(instanceName).RadiusSessionServerConf()

	// If using the default mux (not done here. Just in case...)
	// https://stackoverflow.com/questions/40786526/resetting-http-handlers-in-golang-for-unit-testing
	// http.DefaultServeMux = new(http.ServeMux)
	mux := new(http.ServeMux)

	// Build and initialize the underlying store
	rss := RadiusSessionServer{
		config:               ssConf,
		updateChannel:        make(chan *SessionStoreRequest, 1),
		queryChannel:         make(chan *SessionQueryRequest, 1),
		controlChannel:       make(chan interface{}, 1),
		eventLoopDoneChannel: make(chan struct{}, 1),
		httpDoneChannel:      make(chan struct{}),
		httpServer: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", ssConf.HttpBindAddress, ssConf.HttpBindPort),
			Handler:           mux,
			IdleTimeout:       1 * time.Minute,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
	rss.init(ssConf.Attributes, ssConf.IdAttributes, ssConf.IndexConf, ssConf.ExpirationTime, ssConf.LimboTime)

	mux.HandleFunc("/sessionserver/v1/sessions", rss.getQueryHandler())

	// Start event loop, radius and http servers (this last blocks the call. For this reason is executed
	// in a goroutine)
	go rss.run()

	// The created session server
	return &rss
}

// Graceful shutdown
func (ss *RadiusSessionServer) Close() {

	// Close the http server
	ss.httpServer.Shutdown(context.Background())
	<-ss.httpDoneChannel

	// Set down
	ss.controlChannel <- SessionServerSetDownCommand{}

	// Wait here if necessary
	// TODO: Need to wait for anything?

	// Terminate the event loop
	ss.controlChannel <- SessionServerCloseCommand{}

	<-ss.eventLoopDoneChannel

	// Close channels
	close(ss.controlChannel)
	close(ss.queryChannel)
	close(ss.updateChannel)
}

// Execute the event loop, radius and http servers
func (rss *RadiusSessionServer) run() {

	// Start event loop
	go rss.eventLoop()

	// Instantiate the radius server. It starts operating right after instantiation.
	rss.radiusServer = radiusserver.NewRadiusServer(rss.config.ReceiveFrom, rss.config.RadiusBindAddress, rss.config.RadiusBindPort, func(request *core.RadiusPacket) (*core.RadiusPacket, error) {
		return rss.handlePacket(request)
	})

	// Make sure the certificates exists in the current directory
	certFile, keyFile := core.EnsureCertificates()

	// Will block here
	err := rss.httpServer.ListenAndServeTLS(certFile, keyFile)

	// HttpServer terminated
	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting http handler with: " + err.Error())
	}

	close(rss.httpDoneChannel)
}

// Event loop
func (ss *RadiusSessionServer) eventLoop() {
	for {
		select {
		case cr := <-ss.controlChannel:

			switch cr.(type) {

			case SessionServerSetDownCommand:

				// Just avoid accepting more requests
				ss.status = StatusTerminated

			case SessionServerCloseCommand:

				// Terminate the event loop
				ss.eventLoopDoneChannel <- struct{}{}
				return
			}

			// New session request
		case ur := <-ss.updateChannel:

			// Do not accept if we are terminated
			if ss.status == StatusTerminated {
				ur.RChan <- errors.New("session server terminated")

				// Process the packet. If could not be inserted, return an specific
				// IndexConstraintError with the name of the offending index
			} else if _, offendingIndex := ss.PushPacket(ur.Packet); offendingIndex == "" {
				ur.RChan <- nil
			} else {
				ur.RChan <- IndexConstraintError{offendingIndex}
			}

			// The response channel is closed by the responder
			close(ur.RChan)

		case qr := <-ss.queryChannel:
			// Do not accept if we are terminated
			if ss.status == StatusTerminated {
				qr.RChan <- errors.New("session server terminated")
			} else {
				// Build the response
				resp := SessionQueryResponse{}
				for _, session := range ss.FindByIndex(qr.IndexName, qr.IndexValue, qr.ActiveOnly) {
					resp.Items = append(resp.Items, session.AVPs)
				}
				qr.RChan <- resp
			}

			// The response channel is closed by the responder
			close(qr.RChan)
		}

	}
}

// Pushes entries received via radius to the store
// The first returned value is the packet that must be sent as response in case of an access-accept
// denied due to index constraint not met.
// If error is true, some other kind of error occured and the packet should be dropped
func (rss *RadiusSessionServer) handlePacket(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	sur := SessionStoreRequest{
		RChan:  make(chan error, 1),
		Packet: request,
	}

	// Send request
	rss.updateChannel <- &sur

	// Wait for response
	e := <-sur.RChan

	var constraintError IndexConstraintError
	if e == nil {
		// Processed correctly
		return core.NewRadiusResponse(request, true), nil
	} else if errors.As(e, &constraintError) {
		// Not inserted due to constraint
		return core.NewRadiusResponse(request, false).Add("Reply-Message", "Duplicated entry for index "+constraintError.offendingIndex), nil
	} else {
		// Not inserted due to a generic error
		return nil, e
	}
}

// Executes a query sending and receiving messages from/to the event loop
func (rss *RadiusSessionServer) HandleQuery(indexName string, indexValue string, activeOnly bool) (SessionQueryResponse, error) {

	sqr := SessionQueryRequest{
		RChan:      make(chan interface{}, 1),
		IndexName:  indexName,
		IndexValue: indexValue,
		ActiveOnly: activeOnly,
	}

	// Send request
	rss.queryChannel <- &sqr

	// Wait for response
	resp := <-sqr.RChan
	switch v := resp.(type) {
	case error:
		return SessionQueryResponse{}, v
	case SessionQueryResponse:
		return v, nil
	default:
		panic("Process query got an answer that is not error or *[]RadiusPacket")
	}
}

// Returns the function to handle http queries.
// This way we can parametrize the function if needed.
// sessions?index_name=<value>&index_value=<name>&active_only=[true|false]
func (ss *RadiusSessionServer) getQueryHandler() func(w http.ResponseWriter, req *http.Request) {

	// This is the handler function
	return func(w http.ResponseWriter, req *http.Request) {

		// Get the query index and value
		var indexName string
		var indexValue string
		var activeOnly bool
		qv := req.URL.Query()
		for q := range qv {
			if q == "index_name" {
				indexName = qv.Get(q)
			} else if q == "index_value" {
				indexValue = qv.Get(q)
			} else if q == "active_only" {
				activeOnly, _ = strconv.ParseBool(qv.Get(q))
			}
		}

		// If request is empty, answer with OK
		if indexName == "" || indexValue == "" {
			w.WriteHeader(http.StatusOK)
			core.IncrementSessionQueries(req.URL.Path, "", constants.SUCCESS)
			return
		}

		// Invoke here to get the answer
		response, err := ss.HandleQuery(indexName, indexValue, activeOnly)
		if err != nil {
			treatError(w, err, err.Error(), http.StatusInternalServerError, req.URL.Path, indexName, constants.HANDLER_FUNCTION_ERROR)
			return
		}

		// Parse the answer
		jAnswer, err := json.Marshal(&response)
		if err != nil {
			treatError(w, err, "error marshaling response", http.StatusInternalServerError, req.URL.Path, indexName, constants.UNSERIALIZATION_ERROR)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		core.IncrementSessionQueries(req.URL.Path, indexName, constants.SUCCESS)
	}
}

// Helper function to avoid code duplication
func treatError(w http.ResponseWriter, err error, message string, statusCode int, reqURI string, indexName string, appErrorCode string) {
	core.GetLogger().Errorf(message+": %s", err)
	w.WriteHeader(statusCode)
	w.Write([]byte(err.Error()))
	core.IncrementSessionQueries(reqURI, indexName, appErrorCode)
}
