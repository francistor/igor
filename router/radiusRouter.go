package router

import "time"

// RadiusRouter
// Starts an UDP server socket
//
// Receives RoutableRadiusPacket messages, which contain a radius packet plus the specification of the server to send it to or,
// if empty, handles it. Handling can be done with the registered http handler or with the specified handler function
//
// When sending packets to other radius servers, the router obtains the final radius enpoint by analyzing the radius group,
// and sends the packet to the RadiusClient. It also manages the request-level retries, as oposed to the server-level retries,
// which are managed by the RadiusClientSocket
//
// The status of the radius servers is kept on a table. Radius Server are marked as "down" when the number of timeouts in a row
// exceed the configured value
// Message to be sent for orderly shutdown of the Router

// Keeps the status of the Radius Server
// Only declared servers have status
type RadiusServerWithStatus struct {
	// Pointer to the corresponding DiamterPeer
	ServerName string

	// True when the Peer may admit requests
	IsAvailable bool

	// For reporting purposes
	LastStatusChange time.Time
	LastError        error
}
