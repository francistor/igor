package router

import (
	"crypto/tls"
	"igor/config"
	"igor/radiuscodec"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// RadiusRouter
// Starts an UDP server socket
//
// Receives RoutableRadiusPacket messages, which contain a radius packet plus the specification of the server where to send it.
// If empty, handles it. Handling can be done with the registered http handler or with the specified handler function
//
// When sending packets to other radius servers, the router obtains the final radius enpoint by analyzing the radius group,
// and sends the packet to the RadiusClient. It also manages the request-level retries, as oposed to the server-level retries,
// which are managed by the RadiusClientSocket
//
// The status of the radius servers is kept on a table. Radius Server are marked as "down" when the number of timeouts in a row
// exceed the configured value

// Keeps the status of the Radius Server
// Only declared servers have status
type RadiusServerWithStatus struct {
	// Pointer to the corresponding DiameterPeer
	ServerName string

	// True when the Peer may admit requests
	IsAvailable bool

	// Quarantined time
	UnavailableUntil time.Time

	// For reporting purposes
	LastStatusChange time.Time
	LastError        error
}

// Represents a Radius Packet to be handled or proxyed
//
type RoutableRadiusRequest struct {

	// Can be a radius server group name, a radius
	// server name or an IPaddress:Port
	// If zero, the packet is to be handled locally
	Endpoint string

	// Has a value if the endpoint is an IPAddress:Port
	Secret string

	// Pointer to the actual RadiusPacket
	RadiusPacket *radiuscodec.RadiusPacket

	// The channel to send the answer or error
	RChan chan interface{}

	// Timeout
	Timeout time.Duration
}

type RadiusRouter struct {
	// Configuration instance
	instanceName string

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// Stauts of the Router
	status int32

	// Status of the upstream radius servers declared in the configuration
	radiusServersTable map[string]RadiusServerWithStatus

	// Used to retreive Radius Requests
	radiusRequestsChan chan RoutableRadiusRequest

	// To signal that the Router has shut down
	RouterDoneChannel chan struct{}

	// HTTP2 client
	http2Client http.Client
}

// Creates and runs a Router
func NewRadiusRouter(instanceName string) *RadiusRouter {

	router := RadiusRouter{
		instanceName:       instanceName,
		ci:                 config.GetPolicyConfigInstance(instanceName),
		radiusServersTable: make(map[string]RadiusServerWithStatus),
		radiusRequestsChan: make(chan RoutableRadiusRequest, RADIUS_REQUESTS_QUEUE_SIZE),
		RouterDoneChannel:  make(chan struct{}),
	}

	// Configure client for handlers
	transportCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	router.http2Client = http.Client{Timeout: HTTP_TIMEOUT_SECONDS * time.Second, Transport: transportCfg}

	go router.eventLoop()

	return &router
}

func (router *RadiusRouter) eventLoop() {
}
