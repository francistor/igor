package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"io/ioutil"
	"net/http"
)

// Helper function to serialize, send request, get response and unserialize Diamter Request
func HttpDiameterRequest(client http.Client, endpoint string, diameterRequest *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	// Serialize the message
	jsonRequest, err := json.Marshal(diameterRequest)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal message to json %s", err)
	}

	// Send the request to the Handler
	httpResp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonRequest))
	if err != nil {
		return nil, fmt.Errorf("handler %s error %s", endpoint, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("handler %s returned status code %d", endpoint, httpResp.StatusCode)
	}

	jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response from %s %s", endpoint, err)
	}

	// Unserialize to Diameter Message
	var diameterAnswer diamcodec.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response from %s %s", endpoint, err)
	}

	return &diameterAnswer, nil
}

// To be used in an HTTP server
// Given a Diameter Handler function, builds an http handler that unserializes, executes the handler and serializes the response
func getDiamterRequestHandler(handlerFunc func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error)) func(w http.ResponseWriter, req *http.Request) {

	h := func(w http.ResponseWriter, req *http.Request) {
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

		// Generate the Diameter Answer, invoking the passed function
		answer, err := handlerFunc(&request)
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

	return h
}
