package radiusClient

import (
	"context"
	"fmt"
	"igor/config"
	"igor/instrumentation"
	"igor/radiuscodec"
	"io"
	"net"
	"sync"
	"time"
)

const (
	EVENTLOOP_CAPACITY = 100
)

// The receiverLoop sends this message to the Actor when a packet has been received
type RadiusResponseMsg struct {
	// The sender
	remote net.UDPAddr

	// The received packet
	packetBytes []byte
}

// Sent to the eventLoop in order to send the specified packet
type RadiusRequestMsg struct {
	// Where to send the message to
	endpoint string

	// The packet to send
	packet *radiuscodec.RadiusPacket

	// The response channel
	rchan chan interface{}

	// Timeout
	timeout time.Duration

	// The secret shared with the endpoint
	secret string
}

// Sent to the eventLoop by the timeout handler function
type CancelRequestMsg struct {
	// To identify the resquest
	endpoint string

	// To identify the request
	radiusId byte

	// Will always be "timeout"
	reason error
}

// Send to the event loop to finalize the object
type CloseCommandMsg struct {
}

// Context data for an in flight request
type RequestContext struct {

	// Metric key. Need it because the message will not be available in a timeout
	key instrumentation.RadiusMetricKey

	// Channel on which the answer will be sent
	rchan chan interface{}

	// Timer
	timer *time.Timer

	// Secret
	// Looks like overkill, but this allows sending requests to unconfigured radius servers
	secret string
}

// RadiusClientSocket
// Manages a single UDP port
// Sends radius packets to the upstream servers.
// Keeps track of outstanding requests in a map, which stores the destination endpoint, radiusId, response channel
// and timer, in order to match requests with answers.
type RadiusClientSocket struct {

	// Configuration instance
	ci *config.PolicyConfigurationManager

	// Status
	status int

	// Outstanding requests
	// Nested map. First keyed by destination endpoint (ipaddress:port) and then by radius identifier
	requestsMap map[string]map[byte]RequestContext

	// Last assigned radius id per endpoint
	// Used as a hint for optimization when finding a new id to use
	lastRadiusIdMap map[string]byte

	// UDP socket
	socket net.PacketConn

	// Created iternally. This is for the Actor model loop
	eventLoopChannel chan interface{}

	// Created internaly, for synchronizing the event and read loops
	// The ReadLoop will send a message when exiting, signalling that
	// it will not send more messages to the eventLoopChannel, so it
	// can be closed as far as the ReadLoop is concerned
	readLoopDoneChannel chan bool

	// Context for cancellation
	context context.Context

	// Wait group to be used on each goroutine launched, to make sure that
	// the eventloop channel is not used after being closed
	wg sync.WaitGroup
}

// Creation function
func NewRadiusClientSocket(ci *config.PolicyConfigurationManager, bindIPAddress string, originPort int) *RadiusClientSocket {

	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindIPAddress, originPort))
	if err != nil {
		panic(fmt.Sprintf("could not bind client socket to %s:%d: %s", bindIPAddress, originPort, err))
	}

	rcs := RadiusClientSocket{
		ci:                  ci,
		requestsMap:         make(map[string]map[byte]RequestContext),
		lastRadiusIdMap:     make(map[string]byte),
		eventLoopChannel:    make(chan interface{}, EVENTLOOP_CAPACITY),
		socket:              socket,
		readLoopDoneChannel: make(chan bool),
	}

	go rcs.eventLoop()
	go rcs.readLoop(rcs.readLoopDoneChannel)

	return &rcs
}

// Actor model event loop. All interaction with RadiusClientSocket takes place by
// sending messages which are processed here
func (rcs *RadiusClientSocket) eventLoop() {

	for {

		in := <-rcs.eventLoopChannel

		switch v := in.(type) {

		case CloseCommandMsg:
			rcs.status = StatusClosing

			// Will generate an error in the loop, and the readerLoop will return
			rcs.socket.Close()

		case RadiusResponseMsg:
			// Get the data necessary to get from requests map
			endpoint := v.remote.String()
			radiusId := v.packetBytes[1]
			if epReqMap, ok := rcs.requestsMap[endpoint]; !ok {
				config.GetLogger().Debugf("unsolicited response from endpoint %s", endpoint)
				continue
			} else if reqCtx, ok := epReqMap[radiusId]; !ok {
				config.GetLogger().Debugf("unsolicited response from endpoint %s", endpoint)
				continue
			} else {

				// TODO: Verify authenticator

				// Decode the packet
				radiusPacket, err := radiuscodec.RadiusPacketFromBytes(v.packetBytes, reqCtx.secret)
				if err != nil {
					config.GetLogger().Errorf("error decoding packet %s", err)
				}
				clientIPAddr := v.remote.IP.String()
				instrumentation.PushRadiusClientResponse(clientIPAddr, string(radiusPacket.Code))
				config.GetLogger().Debugf("<- Client received RadiusPacket %s\n", radiusPacket)

				// Cancel timer
				if reqCtx.timer.Stop() {
					// The after func has not been called
					rcs.wg.Done()
				} else {
					// Drain the channel. https://itnext.io/go-timer-101252c45166
					select {
					case <-reqCtx.timer.C:
					default:
					}

				}
				// Send the answer to the requester
				reqCtx.rchan <- radiusPacket
				close(reqCtx.rchan)

				// Remove from outstanding requests
				delete(epReqMap, radiusId)
			}

		case RadiusRequestMsg:

			radiusId, error := rcs.getNextRadiusId(v.endpoint)
			if error != nil {
				v.rchan <- error
				break
			}

			packetBytes, error := v.packet.ToBytes(v.secret, radiusId)
			if error != nil {
				v.rchan <- error
				break
			}

			remoteAddr, _ := net.ResolveUDPAddr("udp", v.endpoint)
			_, err := rcs.socket.WriteTo(packetBytes, remoteAddr)
			if err != nil {
				v.rchan <- error
				break
			}

			// For the timer to be created below
			rcs.wg.Add(1)

			// Set request map and start timer
			rcs.requestsMap[v.endpoint][radiusId] = RequestContext{
				key: instrumentation.RadiusMetricKey{
					Endpoint: v.endpoint,
					Code:     string(v.packet.Code),
				},
				rchan: v.rchan,
				timer: time.AfterFunc(v.timeout, func() {
					// This will be called if the timer expires
					rcs.eventLoopChannel <- CancelRequestMsg{endpoint: v.endpoint, radiusId: radiusId, reason: fmt.Errorf("timeout")}
					defer rcs.wg.Done()
				}),
				secret: v.secret,
			}

		case CancelRequestMsg:

		}
	}

}

// Starts the closing process
func (rcs *RadiusClientSocket) SetDown() {

}

// Loop for receiving answer messages
func (rcs *RadiusClientSocket) readLoop(ch chan bool) {

	// Single buffer where all incoming packets are read
	// According to RFC 2865, the maximum packet size is 4096
	reqBuf := make([]byte, 4096)

	for {
		packetSize, clientAddr, err := rcs.socket.ReadFrom(reqBuf)
		if err != nil {
			if err == io.EOF {
				rcs.eventLoopChannel <- ReadEOFMsg{}
			} else {
				rcs.eventLoopChannel <- ReadErrorMsg{err}
			}
			break
		}

		// Send to eventLoop
		var packetBytes []byte
		copy(packetBytes, reqBuf[:packetSize])

		rcs.eventLoopChannel <- RadiusResponseMsg{
			remote:      *clientAddr.(*net.UDPAddr),
			packetBytes: packetBytes,
		}
	}

	// Signal that we are finished
	close(ch)
}

// Gets the next radiusid to use, or error if all are busy
// Allocates the nested map if not already instantiated
// After calling this function it can be ensured that the requestsMap contains
// a map for the specifed endpoint
func (rcs *RadiusClientSocket) getNextRadiusId(endpoint string) (byte, error) {

	idMap, found := rcs.requestsMap[endpoint]
	if !found {
		// Map for this endpoint does not exist yet. Create it
		rcs.requestsMap[endpoint] = make(map[byte]RequestContext)
	}

	lastId, found := rcs.lastRadiusIdMap[endpoint]
	if !found {
		// Map for this endpoint does not exist yet. Create it
		rcs.lastRadiusIdMap[endpoint] = 0
	}

	// Try to get a radius Id
	nextId := lastId
	for i := 0; i < 255; i++ {
		if nextId == 255 {
			nextId = 0
		} else {
			nextId = nextId + 1
		}
		if _, ok := idMap[nextId]; !ok {
			rcs.lastRadiusIdMap[endpoint] = nextId
			return nextId, nil
		}
	}

	return 0, fmt.Errorf("exhausted ids for endpoint %s", endpoint)

}
