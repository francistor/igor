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

// Implements a session store that
// - Is updated via Radius requests
// - Can be queried by http
// - Communicates with other RadiusSessionServers
//   - Sending copies of the received radius requests
//   - Sending, when requested, a full dump of the store
type RadiusSessionServer struct {
	RadiusSessionStore

	// Holds the radius server
	radiusServer *radiusserver.RadiusServer

	// Holds the httpserver
	httpServer *http.Server

	// For signaling finalization of the http server
	httpDoneChannel chan interface{}
}

// Creates a new instance of the RadiusSessionServer
func NewRadiusSessionServer(instanceName string) *RadiusSessionServer {

	ssConf := core.GetRadiusSessionServerConfigInstance(instanceName).RadiusSessionServerConf()

	// Build the unerlying store
	rss := RadiusSessionServer{}
	rss.init(ssConf.IdAttributes, ssConf.IndexNames, ssConf.ExpirationTime, ssConf.LimboTime)

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
	rss.httpServer.Shutdown(context.Background())
	<-rss.httpDoneChannel
}

// Execute the http server. This function blocks. Should be executed
// in a goroutine.
func (rss *RadiusSessionServer) run() {

	// Make sure the certificates exists in the current directory
	certFile, keyFile := core.EnsureCertificates()

	err := rss.httpServer.ListenAndServeTLS(certFile, keyFile)

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting http handler with: " + err.Error())
	}

	close(rss.httpDoneChannel)
}

// Pushes entries to the store
func (rss *RadiusSessionServer) handlePacket(request *core.RadiusPacket) (*core.RadiusPacket, error) {

	// Always return a success
	return core.NewRadiusResponse(request, true), nil
}

// Returns the function to handle http queries.
// This way we can parametrize the function if needed.
func getQueryHandler() func(w http.ResponseWriter, req *http.Request) {

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
		resp := SessionsResponse{}
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
