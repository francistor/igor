package sessionserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/francistor/igor/constants"
	"github.com/francistor/igor/core"
	"github.com/francistor/igor/radiusclient"
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

// Signal to expire sesions
type SessionPurgeCommand struct {
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

	// Holds the radius client for replication to other session servers
	radiusClient *radiusclient.RadiusClient

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

	// Make sure goroutines are executed before closing
	wg sync.WaitGroup

	// For session expiration
	ticker *time.Ticker
}

// Creates a new instance of the RadiusSessionServer
func NewRadiusSessionServer(instanceName string) *RadiusSessionServer {

	// Get the configuration
	ssConf := core.GetRadiusSessionServerConfigInstance(instanceName).RadiusSessionServerConf()
	ssConf.Attributes = append(ssConf.Attributes, "SessionStore-Expires", "SessionStore-LastUpdated", "SessionStore-Id", "SessionStore-SeenBy")

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

// Execute the event loop, radius and http servers
func (rss *RadiusSessionServer) run() {

	// Start event loop
	go rss.eventLoop()

	// Instantiate the radius server. It starts operating right after instantiation.
	rss.radiusServer = radiusserver.NewRadiusServer(rss.config.ReceiveFrom, rss.config.RadiusBindAddress, rss.config.RadiusBindPort, func(request *core.RadiusPacket) (*core.RadiusPacket, error) {
		return rss.HandlePacket(request)
	})

	// Instantiate the radius client.
	rss.radiusClient = radiusclient.NewRadiusClient()

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

// Graceful shutdown
func (ss *RadiusSessionServer) Close() {

	// Stop the ticker
	if ss.ticker != nil {
		ss.ticker.Stop()
	}

	// Close the http server
	ss.httpServer.Shutdown(context.Background())
	<-ss.httpDoneChannel

	// Set down
	ss.controlChannel <- SessionServerSetDownCommand{}

	// Wait for goroutines to finish
	ss.wg.Wait()

	// Close radius server and client
	ss.radiusServer.Close()
	ss.radiusClient.SetDown()
	ss.radiusClient.Close()

	// Terminate the event loop
	ss.controlChannel <- SessionServerCloseCommand{}

	<-ss.eventLoopDoneChannel

	// Close channels
	close(ss.controlChannel)
	close(ss.queryChannel)
	close(ss.updateChannel)
}

// Event loop
func (ss *RadiusSessionServer) eventLoop() {

	// Sends Ticks through the packet channel, to signal that a write must be
	// done even if the number of packets has not reached the triggering value.
	purgeIntervalMillis := ss.config.PurgeIntervalMillis
	if purgeIntervalMillis == 0 {
		ss.ticker = time.NewTicker(1000 * time.Millisecond)
	} else {
		ss.ticker = time.NewTicker(time.Duration(purgeIntervalMillis) * time.Millisecond)
	}

	for {
		select {

		case <-ss.ticker.C:

			now := time.Now()
			ss.expireAllEntries(now, now)

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

			// Replication
			if ss.status == StatusTerminated {
				ur.RChan <- errors.New("session server terminated")
			} else if ur.Packet.Code == core.ACCOUNTING_REQUEST {
				// Iterate over the sendTo array

			destinationLoop:
				for _, destination := range ss.config.SendTo {
					// Check if we need to send the packet, or that would be a loop
					seenByAttrs := ur.Packet.GetAllAVP("SessionStore-SeenBy")
					for _, avp := range seenByAttrs {
						if avp.GetString() == destination.Name {
							continue destinationLoop
						}
					}
					if (len(seenByAttrs)) > 4 {
						core.GetLogger().Errorf("Session Replica loop aborted. Check configuration")
						continue
					}

					// Choose random port. Specification in serverTo entry has precedence over specification in replicationParams
					var originPorts []int
					if len(destination.OriginPorts) == 0 {
						originPorts = append(originPorts, ss.config.ReplicationParams.OriginPorts...)
					} else {
						originPorts = append(originPorts, destination.OriginPorts...)
					}
					originPort := originPorts[rand.Intn(len(originPorts))]

					ss.wg.Add(1)
					go func(destination core.RadiusServer) {
						defer ss.wg.Done()

						// Generate copy
						packetToSend := ur.Packet.Copy(ss.attributes, nil)

						// Send the radius packet
						// Will only use server tries (not tries, since we are not sending to a group)
						// Replication metrics will be shown as radius client metrics
						ch := make(chan interface{}, 1)
						ss.radiusClient.RadiusExchange(fmt.Sprintf("%s:%d", destination.IPAddress, destination.AcctPort), originPort,
							packetToSend, time.Duration(ss.config.ReplicationParams.TimeoutSecs)*time.Second, ss.config.ReplicationParams.ServerTries,
							destination.Secret, ch)

						// Block here until response or error
						response := <-ch

						// ch was closed in RadiusExchange
						switch v := response.(type) {
						case *core.RadiusPacket:
							// Correctly replicated
						case error:
							core.GetLogger().Warnf("could not replicate session to %s: %s", destination.IPAddress, v.Error())
						}
					}(destination)
				}
			}

		case qr := <-ss.queryChannel:
			// Do not accept if we are terminated
			if ss.status == StatusTerminated {
				qr.RChan <- errors.New("session server terminated")
			} else {
				// Build the response
				resp := SessionQueryResponse{Items: make([][]core.RadiusAVP, 0)} // Initialize to return an empty array instead of a null
				for _, session := range ss.FindByIndex(qr.IndexName, qr.IndexValue, qr.ActiveOnly) {
					resp.Items = append(resp.Items, session.AVPs)
				}
				qr.RChan <- resp
			}

			// The response channel is closed by the responder
			close(qr.RChan)

		} // select

	} // for
}

// Pushes entries received via radius to the store
// The first returned value is the packet that must be sent as response in case of an access-accept
// denied due to index constraint not met.
// If error is true, some other kind of error occured and the packet should be dropped
func (rss *RadiusSessionServer) HandlePacket(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	// Generate the request to the event loop.
	// Add myself as processor of this packet (loop avoidance)
	sur := SessionStoreRequest{
		RChan:  make(chan error, 1),
		Packet: request.Add("SessionStore-SeenBy", rss.config.Name),
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
