package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/francistor/igor/constants"
	"github.com/francistor/igor/core"
)

// Receives Radius & Diameter requests via HTTP2, in JSON format, and processes them with the provided handlers
// The request is converted back to a Radius or Diameter message. So, the whole point of JSON serialization is
// just an overhead. This is used mainly for testing: HTTP handlers are exected to be developed in more JSON
// friendly langages such as javascript.

type HttpHandler struct {
	// Holds the configuration instance for this Handler
	ci *core.HttpHandlerConfigurationManager

	// Holds the httpserver
	httpServer *http.Server

	// For signaling finalization
	doneChannel chan interface{}
}

// Creates a new DiameterHandler object
func NewHttpHandler(instanceName string, diameterHandler core.DiameterMessageHandler, radiusHandler core.RadiusPacketHandler) *HttpHandler {

	// If using the default mux (not done here. Just in case...)
	// https://stackoverflow.com/questions/40786526/resetting-http-handlers-in-golang-for-unit-testing
	// http.DefaultServeMux = new(http.ServeMux)
	mux := new(http.ServeMux)
	mux.HandleFunc("/diameterRequest", getDiameterRequestHandler(diameterHandler))
	mux.HandleFunc("/radiusRequest", getRadiusRequestHandler(radiusHandler))

	ci := core.GetHttpHandlerConfigInstance(instanceName)
	bindAddrPort := fmt.Sprintf("%s:%d", ci.HttpHandlerConf().BindAddress, ci.HttpHandlerConf().BindPort)
	core.GetLogger().Infof("handler listening in %s", bindAddrPort)

	h := HttpHandler{
		ci: ci,
		httpServer: &http.Server{
			Addr:              bindAddrPort,
			Handler:           mux,
			IdleTimeout:       1 * time.Minute,
			ReadHeaderTimeout: 5 * time.Second,
		},
		doneChannel: make(chan interface{}, 1),
	}

	go h.run()

	return &h
}

// Execute the Radius and Diameter handlers. This function blocks. Should be executed
// in a goroutine.
func (dh *HttpHandler) run() {

	// Make sure the certificates exists in the current directory
	certFile, keyFile := core.EnsureCertificates()

	err := dh.httpServer.ListenAndServeTLS(certFile, keyFile)

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting http handler with: " + err.Error())
	}

	close(dh.doneChannel)
}

// Gracefully shutdown
func (dh *HttpHandler) Close() {
	dh.httpServer.Shutdown(context.Background())
	<-dh.doneChannel
}

// Given a Diameter Handler function, builds a http handler that unserializes, executes the handler and serializes the response
func getDiameterRequestHandler(handlerFunc core.DiameterMessageHandler) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {

		// Get the Diameter Request
		jRequest, err := io.ReadAll(req.Body)
		if err != nil {
			treatError(w, err, "error reading request", http.StatusInternalServerError, req.RequestURI, constants.NETWORK_ERROR)
			return
		}
		var request core.DiameterMessage
		if err = json.Unmarshal(jRequest, &request); err != nil {
			treatError(w, err, "error unmarshaling request", http.StatusBadRequest, req.RequestURI, constants.UNSERIALIZATION_ERROR)
			return
		}
		request.Tidy()

		// Generate the Diameter Answer, invoking the passed function
		answer, err := handlerFunc(&request)
		if err != nil {
			treatError(w, err, "error handling request", http.StatusInternalServerError, req.RequestURI, constants.HANDLER_FUNCTION_ERROR)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			treatError(w, err, "error marshaling response", http.StatusInternalServerError, req.RequestURI, constants.SERIALIZATION_ERROR)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		core.RecordHttpHandlerExchange(req.RequestURI, constants.SUCCESS)
	}
}

// Given a Diameter Handler function, builds an http handler that unserializes, executes the handler and serializes the response
func getRadiusRequestHandler(handlerFunc core.RadiusPacketHandler) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {

		// Get the Radius Request
		jRequest, err := io.ReadAll(req.Body)
		if err != nil {
			treatError(w, err, "error reading request", http.StatusInternalServerError, req.RequestURI, constants.NETWORK_ERROR)
			return
		}
		var request core.RadiusPacket
		if err = json.Unmarshal(jRequest, &request); err != nil {
			treatError(w, err, "error unmarshaling request", http.StatusBadRequest, req.RequestURI, constants.UNSERIALIZATION_ERROR)
			return
		}

		// Generate the Radius Answer, invoking the passed function
		answer, err := handlerFunc(&request)
		if err != nil {
			treatError(w, err, "error marshaling response", http.StatusInternalServerError, req.RequestURI, constants.HANDLER_FUNCTION_ERROR)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			treatError(w, err, "error marshaling response", http.StatusInternalServerError, req.RequestURI, constants.SERIALIZATION_ERROR)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		core.RecordHttpHandlerExchange(req.RequestURI, constants.SUCCESS)
	}
}

// Helper function to avoid code duplication
func treatError(w http.ResponseWriter, err error, message string, statusCode int, reqURI string, appErrorCode string) {
	core.GetLogger().Errorf(message+": %s", err)
	w.WriteHeader(statusCode)
	w.Write([]byte(err.Error()))
	core.RecordHttpHandlerExchange(reqURI, appErrorCode)
}
