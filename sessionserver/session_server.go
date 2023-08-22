package sessionserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/francistor/igor/constants"
	"github.com/francistor/igor/core"
	"github.com/francistor/igor/radiusserver"
)

type SessionQueryRequest struct {
	RChan      chan interface{}
	IndexName  string
	IndexValue string
	ActiveOnly bool
}

type SessionQueryResponse struct {
	Items [][]core.RadiusAVP
}

type IndexConstraintError struct {
	offendingIndex string
}

func (e IndexConstraintError) Error() string {
	return e.offendingIndex
}

type SessionUpdateRequest struct {
	RChan  chan error
	Packet *core.RadiusPacket
}

// Signal to initiate the termination process
type SessionServerSetDownCommand struct {
}

// Signal to terminate the event loop
type SessionServerCloseCommand struct {
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

	// Holds the radius server
	radiusServer *radiusserver.RadiusServer

	// Holds the httpserver
	httpServer *http.Server

	// For signaling finalization of the http server
	httpDoneChannel chan interface{}

	// For sending session updates
	updateChannel chan *SessionUpdateRequest

	// For sending queries
	queryChannel chan *SessionQueryRequest

	// Event loop control channel
	controlChannel chan interface{}
}

// Creates a new instance of the RadiusSessionServer
func NewRadiusSessionServer(instanceName string) *RadiusSessionServer {

	ssConf := core.GetRadiusSessionServerConfigInstance(instanceName).RadiusSessionServerConf()

	// Build the unerlying store
	rss := RadiusSessionServer{}
	rss.init(ssConf.IdAttributes, ssConf.IndexConf, ssConf.ExpirationTime, ssConf.LimboTime)

	// Create channels
	rss.updateChannel = make(chan *SessionUpdateRequest, 1)
	rss.queryChannel = make(chan *SessionQueryRequest, 1)
	rss.controlChannel = make(chan interface{}, 1)

	// If using the default mux (not done here. Just in case...)
	// https://stackoverflow.com/questions/40786526/resetting-http-handlers-in-golang-for-unit-testing
	// http.DefaultServeMux = new(http.ServeMux)
	mux := new(http.ServeMux)
	mux.HandleFunc("/sessionserver/v1/sessions", getQueryHandler())

	// Instantiate the radius server. It starts operating right after instantiation.
	rss.radiusServer = radiusserver.NewRadiusServer(ssConf.ReceiveFrom, ssConf.RadiusBindAddress, ssConf.RadiusBindPort, func(request *core.RadiusPacket) (*core.RadiusPacket, error) {
		return rss.handlePacket(request)
	})

	go rss.run()

	return &rss
}

// Graceful shutdown
func (rss *RadiusSessionServer) Close() {

	// Close the http server
	rss.httpServer.Shutdown(context.Background())
	<-rss.httpDoneChannel

	// Set down
	rss.controlChannel <- SessionServerSetDownCommand{}

	// Wait here if necessary
	// TODO: Need to wait for anything?

	// Terminate the event loop
	rss.controlChannel <- SessionServerCloseCommand{}

	// Close channels
	close(rss.controlChannel)
	close(rss.queryChannel)
	close(rss.updateChannel)
}

// Execute the http server. This function blocks. Should be executed
// in a goroutine.
func (rss *RadiusSessionServer) run() {

	// Make sure the certificates exists in the current directory
	certFile, keyFile := core.EnsureCertificates()

	// Start event loop
	go rss.eventLoop()

	err := rss.httpServer.ListenAndServeTLS(certFile, keyFile)

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting http handler with: " + err.Error())
	}

	close(rss.httpDoneChannel)
}

// Event loop
func (rss *RadiusSessionServer) eventLoop() {
	for {
		select {
		case cr := <-rss.controlChannel:

			switch cr.(type) {

			case SessionServerSetDownCommand:
				rss.status = StatusTerminated

			case SessionServerCloseCommand:
				return
			}

		case ur := <-rss.updateChannel:
			if rss.status == StatusTerminated {
				ur.RChan <- errors.New("session server terminated")
			} else if _, offendingIndex := rss.PushPacket(ur.Packet); offendingIndex == "" {
				ur.RChan <- nil
			} else {
				ur.RChan <- IndexConstraintError{offendingIndex}
			}

			close(ur.RChan)

		case qr := <-rss.queryChannel:
			if rss.status == StatusTerminated {
				qr.RChan <- errors.New("session server terminated")
			} else {
				qr.RChan <- rss.FindByIndex(qr.IndexName, qr.IndexValue, true)
			}

			close(qr.RChan)
		}

	}
}

// Pushes entries received via radius to the store
func (rss *RadiusSessionServer) handlePacket(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	var constraintError IndexConstraintError

	e := rss.ProcessPacket(request)

	if e == nil {
		return core.NewRadiusResponse(request, true), nil
	} else if errors.As(e, &constraintError) {
		return core.NewRadiusResponse(request, false).Add("Reply-Message", "Duplicated entry for index "+constraintError.offendingIndex), nil
	} else {
		return nil, e
	}
}

// Processes the received request
func (rss *RadiusSessionServer) ProcessPacket(request *core.RadiusPacket) error {

	sur := SessionUpdateRequest{
		RChan:  make(chan error),
		Packet: request,
	}

	// Send request
	rss.updateChannel <- &sur

	// Wait for response
	return <-sur.RChan
}

// Executes query
func (rss *RadiusSessionServer) ProcessQuery(indexName string, indexValue string, activeOnly bool) ([]*core.RadiusPacket, error) {

	sqr := SessionQueryRequest{
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
		return nil, v
	case []*core.RadiusPacket:
		return v, nil
	default:
		panic("Process query got an answer that is not error or *[]RadiusPacket")
	}
}

// Returns the function to handle http queries.
// This way we can parametrize the function if needed.
func getQueryHandler() func(w http.ResponseWriter, req *http.Request) {

	// This is the handler function
	return func(w http.ResponseWriter, req *http.Request) {

		// Get the query index and value
		var indexName string
		var indexValue string
		qv := req.URL.Query()
		for q := range qv {
			indexName = q
			indexValue = qv.Get(q)
		}

		// If request is empty, answer with OK
		if indexName == "" || indexValue == "" {
			w.WriteHeader(http.StatusOK)
			core.IncrementSessionQueries(req.URL.Path, "", constants.SUCCESS)
			return
		}

		// Invoke here to get the answer
		resp := SessionQueryResponse{}
		jAnswer, err := json.Marshal(resp)
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
