package httphandler

import (
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"net/http"
)

type HttpHandler struct {
	// Holds the configuration instance for this Handler
	ci *config.HandlerConfigurationManager
}

// Creates a new DiameterHandler object
func NewHttpHandler(instanceName string, handler func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error)) HttpHandler {
	h := HttpHandler{ci: config.GetHandlerConfigInstance(instanceName)}

	http.HandleFunc("/diameterRequest", getDiamterRequestHandler(handler))

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

/*
func handleDiameterRequest2(w http.ResponseWriter, req *http.Request) {

	logger := config.GetLogger()

	// Get the Diameter Request
	jRequest, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Error("error reading request %s", err)
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var request diamcodec.DiameterMessage
	json.Unmarshal(jRequest, &request)

	// Generate the Diameter Answer
	answer, err := handlerfunctions.EmptyHandler(&request)
	if err != nil {
		logger.Errorf("error handling request %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	jAnswer, err := json.Marshal(answer)
	if err != nil {
		logger.Errorf("error marshaling response %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(jAnswer)
	w.WriteHeader(http.StatusOK)
}
*/
