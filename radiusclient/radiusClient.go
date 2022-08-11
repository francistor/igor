package radiusclient

import (
	"fmt"
	"igor/config"
	"igor/radiuscodec"
	"time"
)

const (
	CONTROL_QUEUE_SIZE  = 16
	REQUESTS_QUEUE_SIZE = 100
	EVENTLOOP_CAPACITY  = 100
)

// Valid statuses
const (
	StatusTerminated = 1
)

// Specification of the Radius packet to send and associated metadata
type ClientRadiusRequestMsg struct {

	// Where to send the message to
	endpoint string

	// Origin port. 0 if unspecified
	originPort int

	// The packet to send
	packet *radiuscodec.RadiusPacket

	// Timeout
	timeout time.Duration

	// Retries
	serverTries int

	// If not 0, reuse instead of allocate, because it is a re-transmission
	radiusId byte

	// The secret shared with the endpoint
	secret string

	// The response channel
	rchan chan interface{}
}

// RadiusClient
//
// Presents a method for sending requests to upstreams servers
// Maintains a set of RadiusClientSockets that own the UDP socket and actually send the requests and receive the answers
// The RadiusClientSockets are created on demand. It is the RadiusRouter which is in control of the origin port used
type RadiusClient struct {

	// Configuration instance
	ci *config.PolicyConfigurationManager

	// Receives events from the RadiusClientSockets and from the external world
	controlChannel chan interface{}

	// For receiving the requests to send
	requestsChannel chan interface{}

	// To signal termination
	doneChannel chan interface{}

	// Map of created RadiusClientSockets by origin port
	clientSockets map[int]*RadiusClientSocket

	// Status may be StatusClosing
	status int32
}

// Creates a new instance of the Radius Client
func NewRadiusClient(ci *config.PolicyConfigurationManager) *RadiusClient {

	rc := RadiusClient{
		ci:              ci,
		controlChannel:  make(chan interface{}, CONTROL_QUEUE_SIZE),
		requestsChannel: make(chan interface{}, REQUESTS_QUEUE_SIZE),
		doneChannel:     make(chan interface{}, 1),
		clientSockets:   make(map[int]*RadiusClientSocket),
	}

	go rc.eventLoop()

	return &rc
}

func (r *RadiusClient) SetDown() {

	// Send myself the message
	r.controlChannel <- SetDownCommandMsg{}
}

// Wait until everything is closed
func (r *RadiusClient) Close() {
	<-r.doneChannel
}

func (r *RadiusClient) eventLoop() {

	for {
		select {
		case m := <-r.controlChannel:
			switch v := m.(type) {
			// RadiusClientSocket reported it is down
			case SocketDownEvent:
				// Close and delete from map
				rcs := v.Sender
				delete(r.clientSockets, rcs.port)

				// While the socket is closed, another one may be created and assigned to the map
				go v.Sender.Close()

				// Check if we are completely finished
				if r.status == StatusTerminated && len(r.clientSockets) == 0 {
					close(r.requestsChannel)
					close(r.controlChannel)
					close(r.doneChannel)
					config.GetLogger().Info("radius client closed")
					return
				}

			case SetDownCommandMsg:

				r.status = StatusTerminated

				// If no clients, we are done
				if len(r.clientSockets) == 0 {
					close(r.requestsChannel)
					close(r.controlChannel)
					close(r.doneChannel)
					config.GetLogger().Info("radius client closed")
					return
				} else {

					// Terminate all radius client sockets. Will terminate when all sockets are down
					for i := range r.clientSockets {
						r.clientSockets[i].SetDown()
					}
				}

			default:
				panic(fmt.Sprintf("unknown message type over RadiusClient control channel %T", v))
			}

		case m := <-r.requestsChannel:
			switch v := m.(type) {
			case ClientRadiusRequestMsg:

				// Do not serve new requests if terminating
				if r.status == StatusTerminated {
					v.rchan <- fmt.Errorf("radius client terminating")
					close(v.rchan)
					continue
				}

				// Check if there is a RadiusClientSocket and create it otherwise
				var rcs *RadiusClientSocket
				var found bool
				if rcs, found = r.clientSockets[v.originPort]; !found {
					rcs = NewRadiusClientSocket(r.ci, r.controlChannel, config.GetPolicyConfig().RadiusServerConf().BindAddress, v.originPort)
					r.clientSockets[v.originPort] = rcs
				}

				// Invoke the operation
				rcs.SendRadiusRequest(v)
			}
		}
	}
}

// Send the radius packet to the target socket and receive the answer or error in the specified channel
func (r *RadiusClient) RadiusExchange(endpoint string, originPort int, packet *radiuscodec.RadiusPacket, timeout time.Duration, serverTries int, secret string, rchan chan interface{}) {

	// Send myself the message
	r.requestsChannel <- ClientRadiusRequestMsg{
		endpoint:    endpoint,
		originPort:  originPort,
		packet:      packet,
		timeout:     timeout,
		serverTries: serverTries,
		secret:      secret,
		rchan:       rchan,
	}
}
