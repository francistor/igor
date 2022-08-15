package router

import (
	"fmt"
	"igor/diamcodec"
	"igor/radiuscodec"
	"strconv"
	"strings"
	"time"
)

// Statuses of the Router
const (
	StatusOperational = int32(0)
	StatusTerminated  = int32(1)
)

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const RADIUS_REQUESTS_QUEUE_SIZE = 16

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const DIAMETER_REQUESTS_QUEUE_SIZE = 16

// Size of the channel for getting peer control messages
// TODO: Anything other than 0 or 1 should be explained
const CONTROL_QUEUE_SIZE = 16

// Timeout in seconds for http2 handlers
const HTTP_TIMEOUT_SECONDS = 10

// TIcker for Diameter Peer checking
const PEER_CHECK_INTERVAL_SECONDS = 60

// Default timeout for requests, when not specified in the origin of the request
// (e.g. diameter request that is routed to another peer instead of being handled)
const DEFAULT_REQUEST_TIMEOUT_SECONDS = 10

// Represents a Diameter Message to be routed, either to a handler
// or to another Diameter Peer
type RoutableDiameterRequest struct {
	// Pointer to the actual Diameter message
	Message *diamcodec.DiameterMessage

	// The channel to send the answer or error
	RChan chan interface{} `json:"-"`

	// Timeout
	Timeout time.Duration `json:"-"`

	// Timeout in string format, for JSON encoding
	// Format is <number><units> where
	// <units> may be "s" for seconds and "ms" for milliseconds
	TimeoutSpec string
}

// Represents a Radius Packet to be handled or proxyed
type RoutableRadiusRequest struct {

	// Can be a radius server group name or an <IPaddress>:<Port>
	// If zero, the packet is to be handled locally
	Destination string

	// Has a value if the endpoint is an IPAddress:Port
	Secret string

	// Pointer to the actual RadiusPacket
	Packet *radiuscodec.RadiusPacket

	// The channel to send the answer or error
	RChan chan interface{} `json:"-"`

	// Timeout
	PerRequestTimeout time.Duration `json:"-"`

	// Timeout in string format, for JSON encoding
	// Format is <number><units> where
	// <units> may be "s" for seconds and "ms" for milliseconds
	PerRequestTimeoutSpec string

	// Number of tries. Should be higher than 0
	Tries int

	// Tries per single server. Should be higher than 0
	ServerTries int
}

/*
Functions to parse the timeout from JSON
*/

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
