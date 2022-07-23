package router

// Statuses of the Router
const (
	StatusOperational = int32(0)
	StatusClosing     = int32(1)
)

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const RADIUS_REQUESTS_QUEUE_SIZE = 16

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const DIAMETER_REQUESTS_QUEUE_SIZE = 16

// Size of the channel for getting peer control messages
// TODO: Anything other than 0 or 1 should be explained
const PEER_CONTROL_QUEUE_SIZE = 16

// Timeout in seconds for http2 handlers
const HTTP_TIMEOUT_SECONDS = 10

// Message to be sent for orderly shutdown of the Router
type RouterCloseCommand struct {
}
