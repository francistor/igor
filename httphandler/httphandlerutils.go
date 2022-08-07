package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/diamcodec"
	"igor/instrumentation"
	"igor/radiuscodec"
	"io/ioutil"
	"net/http"
)

const (
	SERIALIZATION_ERROR    = "550"
	NETWORK_ERROR          = "551"
	HTTP_RESPONSE_ERROR    = "552"
	HANDLER_FUNCTION_ERROR = "553"
	UNSERIALIZATION_ERROR  = "554"

	SUCCESS = "200"
)

// Helper function to serialize, send request, get response and unserialize Diameter Request
func HttpDiameterRequest(client http.Client, endpoint string, diameterRequest *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	// Serialize the message
	jsonRequest, err := json.Marshal(diameterRequest)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, SERIALIZATION_ERROR)
		return nil, fmt.Errorf("unable to marshal message to json %s", err)
	}

	// Send the request to the Handler
	httpResp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonRequest))
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, NETWORK_ERROR)
		return nil, fmt.Errorf("handler %s error %s", endpoint, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		instrumentation.PushHttpClientExchange(endpoint, HTTP_RESPONSE_ERROR)
		return nil, fmt.Errorf("handler %s returned status code %d", endpoint, httpResp.StatusCode)
	}

	jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, NETWORK_ERROR)
		return nil, fmt.Errorf("error reading response from %s: %s", endpoint, err)
	}

	// Unserialize to Diameter Message
	var diameterAnswer diamcodec.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, UNSERIALIZATION_ERROR)
		return nil, fmt.Errorf("error unmarshaling response from %s: %s", endpoint, err)
	}

	instrumentation.PushHttpClientExchange(endpoint, SUCCESS)
	return &diameterAnswer, nil
}

// Helper function to serialize, send request, get response and unserialize Radius Request
func HttpRadiusRequest(client http.Client, endpoint string, packet *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	// Serialize the message
	jsonRequest, err := json.Marshal(packet)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, SERIALIZATION_ERROR)
		return nil, fmt.Errorf("unable to marshal message to json %s", err)
	}

	// Send the request to the Handler
	httpResp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonRequest))
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, NETWORK_ERROR)
		return nil, fmt.Errorf("handler %s error %s", endpoint, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		instrumentation.PushHttpClientExchange(endpoint, HTTP_RESPONSE_ERROR)
		return nil, fmt.Errorf("handler %s returned status code %d", endpoint, httpResp.StatusCode)
	}

	jsonResponse, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, NETWORK_ERROR)
		return nil, fmt.Errorf("error reading response from %s: %s", endpoint, err)
	}

	// Unserialize to Radius Packet
	var radiusResponse radiuscodec.RadiusPacket
	err = json.Unmarshal(jsonResponse, &radiusResponse)
	if err != nil {
		instrumentation.PushHttpClientExchange(endpoint, UNSERIALIZATION_ERROR)
		return nil, fmt.Errorf("error unmarshaling response from %s: %s", endpoint, err)
	}

	instrumentation.PushHttpClientExchange(endpoint, SUCCESS)
	return &radiusResponse, nil
}
