package httphandler

import (
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"igor/instrumentation"
	"io/ioutil"
	"net/http"
)

type HttpHandler struct {
	// Holds the configuration instance for this Handler
	ci *config.HandlerConfigurationManager
}

// Creates a new DiameterHandler object
func NewHttpHandler(instanceName string, handler diampeer.MessageHandler) HttpHandler {
	h := HttpHandler{ci: config.GetHandlerConfigInstance(instanceName)}

	http.HandleFunc("/diameterRequest", getDiameterRequestHandler(handler))

	// TODO: Close gracefully
	go h.Run()
	return h
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *HttpHandler) Run() {

	logger := config.GetLogger()

	bindAddrPort := fmt.Sprintf("%s:%d", dh.ci.HandlerConf().BindAddress, dh.ci.HandlerConf().BindPort)

	logger.Infof("listening in %s", bindAddrPort)
	http.ListenAndServeTLS(bindAddrPort, "/home/francisco/cert.pem", "/home/francisco/key.pem", nil)
}

// Given a Diameter Handler function, builds an http handler that unserializes, executes the handler and serializes the response
func getDiameterRequestHandler(handlerFunc diampeer.MessageHandler) func(w http.ResponseWriter, req *http.Request) {

	h := func(w http.ResponseWriter, req *http.Request) {
		logger := config.GetLogger()

		// Get the Diameter Request
		jRequest, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Error("error reading request %s", err)
			w.Write([]byte(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			instrumentation.PushHttpHandlerExchange(NETWORK_ERROR)
			return
		}
		var request diamcodec.DiameterMessage
		if err = json.Unmarshal(jRequest, &request); err != nil {
			logger.Error("error unmarshalling request %s", err)
			w.Write([]byte(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			instrumentation.PushHttpHandlerExchange(UNSERIALIZATION_ERROR)
			return
		}

		// Generate the Diameter Answer, invoking the passed function
		answer, err := handlerFunc(&request)
		if err != nil {
			logger.Errorf("error handling request %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpHandlerExchange(HANDLER_FUNCTION_ERROR)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			logger.Errorf("error marshaling response %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpHandlerExchange(SERIALIZATION_ERROR)
			return
		}
		w.Write(jAnswer)
		w.WriteHeader(http.StatusOK)
	}

	instrumentation.PushHttpHandlerExchange(SUCCESS)
	return h
}
