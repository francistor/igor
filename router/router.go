package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/francistor/igor/constants"
	"github.com/francistor/igor/core"
)

// Statuses of the Router
const (
	StatusOperational = int32(0)
	StatusTerminated  = int32(1)
)

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const RADIUS_REQUESTS_QUEUE_SIZE = 64

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const DIAMETER_REQUESTS_QUEUE_SIZE = 64

// Size of the channel for getting peer control messages
// TODO: Anything other than 0 or 1 should be explained
const CONTROL_QUEUE_SIZE = 16

// Timeout in seconds for http2 handlers
const HTTP_TIMEOUT_SECONDS = 10

// TIcker for Diameter Peer checking
const DEFAULT_PEER_CHECK_INTERVAL_SECONDS = 120

// Default timeout for requests, when not specified in the origin of the request
// (e.g. diameter request that is routed to another peer instead of being handled)
const DEFAULT_REQUEST_TIMEOUT_SECONDS = 10

// Represents a Diameter Message to be routed, either to a handler
// or to another Diameter Peer
type RoutableDiameterRequest struct {
	// Pointer to the actual Diameter message
	Message *core.DiameterMessage

	// Timeout in string format, for JSON encoding
	// Format is <number><units> where
	// <units> may be "s" for seconds and "ms" for milliseconds
	TimeoutSpec string

	// The channel to send the answer or error
	RChan chan interface{} `json:"-"`

	// Timeout
	Timeout time.Duration `json:"-"`
}

// Represents a Radius Packet to be handled or proxyed
type RoutableRadiusRequest struct {

	// Can be a radius server group name or an <IPaddress>:<Port>
	// If zero, the packet is to be handled locally
	Destination string

	// Has a value if the endpoint is an IPAddress:Port
	Secret string

	// Pointer to the actual RadiusPacket
	Packet *core.RadiusPacket

	// Timeout in string format, for JSON encoding
	// Format is <number><units> where
	// <units> may be "s" for seconds and "ms" for milliseconds
	PerRequestTimeoutSpec string

	// Number of tries. Should be higher than 0
	Tries int

	// Tries per single server. Should be higher than 0
	ServerTries int

	// The channel to send the answer or error
	RChan chan interface{} `json:"-"`

	// Timeout
	PerRequestTimeout time.Duration `json:"-"`
}

/*
Functions to parse the timeout from JSON
*/

// Gets a string as a number followed by "s" or "ms" and
// returns a duration value, as found in a serialized
// Radius or Diameter Routable request
func parseTimeout(timeoutSpec string) (time.Duration, error) {

	if before, _, found := strings.Cut(timeoutSpec, "s"); found {
		if seconds, err := strconv.ParseInt(before, 10, 32); err != nil {
			return 0, err
		} else {
			return time.Duration(seconds) * time.Second, nil
		}
	}

	if before, _, found := strings.Cut(timeoutSpec, "ms"); found {
		if millis, err := strconv.ParseInt(before, 10, 32); err != nil {
			return 0, err
		} else {
			return time.Duration(millis) * time.Millisecond, nil
		}
	}

	return 0, fmt.Errorf("bad timespec format")
}

// Fills the timeout parameter with the specified in the timeoutspec, which is
// unserializable from JSON
func (rdr *RoutableDiameterRequest) ParseTimeout() error {
	if duration, err := parseTimeout(rdr.TimeoutSpec); err != nil {
		return err
	} else {
		rdr.Timeout = duration
		return nil
	}
}

// Fills the timeout parameter with the specified in the timeoutspec, which is
// unserializable from JSON
func (rrr *RoutableRadiusRequest) ParseTimeout() error {
	if duration, err := parseTimeout(rrr.PerRequestTimeoutSpec); err != nil {
		return err
	} else {
		rrr.PerRequestTimeout = duration
		return nil
	}
}

// Message to be sent for orderly shutdown of the Router
type RouterSetDownCommand struct {
}

// Mesaage to stop the eventloop of the routers
type RouterCloseCommand struct {
}

// Helper function to serialize, send request, get response and unserialize Diameter Request
func HttpDiameterRequest(client http.Client, endpoint string, diameterRequest *core.DiameterMessage) (*core.DiameterMessage, error) {
	// Serialize the message
	jsonRequest, err := json.Marshal(diameterRequest)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.SERIALIZATION_ERROR)
		return nil, fmt.Errorf("unable to marshal message to json %s", err)
	}

	// Send the request to the Handler
	httpResp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonRequest))
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.NETWORK_ERROR)
		return nil, fmt.Errorf("handler %s error %s", endpoint, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		core.IncrementHttpClientExchange(endpoint, constants.HTTP_RESPONSE_ERROR)
		return nil, fmt.Errorf("handler %s returned status code %d", endpoint, httpResp.StatusCode)
	}

	jsonAnswer, err := io.ReadAll(httpResp.Body)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.NETWORK_ERROR)
		return nil, fmt.Errorf("error reading response from %s: %s", endpoint, err)
	}

	// Unserialize to Diameter Message
	var diameterAnswer core.DiameterMessage
	err = json.Unmarshal(jsonAnswer, &diameterAnswer)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.UNSERIALIZATION_ERROR)
		return nil, fmt.Errorf("error unmarshaling response from %s: %s", endpoint, err)
	}
	diameterAnswer.Tidy()

	core.IncrementHttpClientExchange(endpoint, constants.SUCCESS)
	return &diameterAnswer, nil
}

// Helper function to serialize, send request, get response and unserialize Radius Request
func HttpRadiusRequest(client http.Client, endpoint string, packet *core.RadiusPacket) (*core.RadiusPacket, error) {
	// Serialize the message
	jsonRequest, err := json.Marshal(packet)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.SERIALIZATION_ERROR)
		return nil, fmt.Errorf("unable to marshal message to json %s", err)
	}

	// Send the request to the Handler
	httpResp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonRequest))
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.NETWORK_ERROR)
		return nil, fmt.Errorf("handler %s error %s", endpoint, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		core.IncrementHttpClientExchange(endpoint, constants.HTTP_RESPONSE_ERROR)
		return nil, fmt.Errorf("handler %s returned status code %d", endpoint, httpResp.StatusCode)
	}

	jsonResponse, err := io.ReadAll(httpResp.Body)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.NETWORK_ERROR)
		return nil, fmt.Errorf("error reading response from %s: %s", endpoint, err)
	}

	// Unserialize to Radius Packet
	var radiusResponse core.RadiusPacket
	err = json.Unmarshal(jsonResponse, &radiusResponse)
	if err != nil {
		core.IncrementHttpClientExchange(endpoint, constants.UNSERIALIZATION_ERROR)
		return nil, fmt.Errorf("error unmarshaling response from %s: %s", endpoint, err)
	}

	core.IncrementHttpClientExchange(endpoint, constants.SUCCESS)
	return &radiusResponse, nil
}
