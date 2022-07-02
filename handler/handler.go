package handler

import (
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"io/ioutil"
	"net/http"
)

type Handler struct {
	// Holds the configuration instance for this Handler
	ci *config.HandlerConfigurationManager
}

// Creates a new DiameterHandler object
func NewHandler(instanceName string) Handler {
	h := Handler{ci: config.GetHandlerConfigInstance(instanceName)}

	http.HandleFunc("/diameterRequest", handleDiameterRequest)

	// TODO: Close gracefully
	go h.Run()
	return h
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *Handler) Run() {

	logger := config.GetLogger()

	bindAddrPort := fmt.Sprintf("%s:%d", dh.ci.HandlerConf().BindAddress, dh.ci.HandlerConf().BindPort)

	logger.Infof("listening in %s", bindAddrPort)
	http.ListenAndServeTLS(bindAddrPort, "/home/francisco/cert.pem", "/home/francisco/key.pem", nil)

}

func handleDiameterRequest(w http.ResponseWriter, req *http.Request) {

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
	answer, err := EmptyHandler(request)
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
