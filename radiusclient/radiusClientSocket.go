package radiusClient

import (
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

// Sent to the RadiusClient when the connection and the eventloop are terminated, due
// to a request or an error
// The RadiusClient can then invoke the Close() command
type SocketDownEvent struct {
	// Myself
	Sender *RadiusClientSocket

	// Will be nil if the reason is not an error
	Error error
}

//////////////////////////////////////////////////////////////////////////////
// Eventloop messages
//////////////////////////////////////////////////////////////////////////////

// The receiverLoop sends this message to the evnntLoop when a packet has been received
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

// Sent when the packet listener reports that the socket has
// been closed
type ReadEOFMsg struct {
}

// General error reading from the socket
type ReadErrorMsg struct {
	Error error
}

//////////////////////////////////////////////////////////////////////////////////

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

	// Authenticator
	authenticator [16]byte
}

// RadiusClientSocket
// Manages a single UDP port
// Sends radius packets to the upstream servers.
// Keeps track of outstanding requests in a map, which stores the destination endpoint, radiusId, response channel
// and timer, in order to match requests with answers.
type RadiusClientSocket struct {

	// Configuration instance
	ci *config.PolicyConfigurationManager

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

	// Passed as parameter. To report events back to the Router
	routerControlChannel chan interface{}

	// Wait group to be used on each goroutine launched, to make sure that
	// the eventloop channel is not used after being closed
	wg sync.WaitGroup
}

// Creation function
func NewRadiusClientSocket(controlChannel chan interface{}, ci *config.PolicyConfigurationManager, bindIPAddress string, originPort int) *RadiusClientSocket {

	// Bind socket
	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindIPAddress, originPort))
	if err != nil {
		panic(fmt.Sprintf("could not bind client socket to %s:%d: %s", bindIPAddress, originPort, err))
	}

	rcs := RadiusClientSocket{
		ci:                   ci,
		requestsMap:          make(map[string]map[byte]RequestContext),
		lastRadiusIdMap:      make(map[string]byte),
		eventLoopChannel:     make(chan interface{}, EVENTLOOP_CAPACITY),
		readLoopDoneChannel:  make(chan bool),
		routerControlChannel: controlChannel,
		socket:               socket,
	}

	go rcs.eventLoop()
	go rcs.readLoop(rcs.readLoopDoneChannel)

	return &rcs
}

// Starts the closing process
func (rcs *RadiusClientSocket) SetDown() {
	config.GetLogger().Debugf("client socket for %d terminating", rcs.socket.LocalAddr().String())

	rcs.eventLoopChannel <- CloseCommandMsg{}
}

// Closes the event loop channel
// Use this method only after a PeerDown event has been received
// Takes some time to execute
func (rcs *RadiusClientSocket) Close() {

	// Wait for the readLoop to stop
	if rcs.readLoopDoneChannel != nil {
		<-rcs.readLoopDoneChannel
	}

	// Wait until all goroutines exit
	rcs.wg.Wait()

	close(rcs.eventLoopChannel)

	config.GetLogger().Debugf("RadiusClientSocket closed")
}

// Actor model event loop. All interaction with RadiusClientSocket takes place by
// sending messages which are processed here
func (rcs *RadiusClientSocket) eventLoop() {

	defer func() {
		// No harm to do it twice
		rcs.socket.Close()
	}()

	for {

		in := <-rcs.eventLoopChannel

		switch v := in.(type) {

		case ReadEOFMsg:

			rcs.cancelAll()

			// Tell the router we are down
			rcs.routerControlChannel <- SocketDownEvent{Sender: rcs}

			return

		case ReadErrorMsg:

			rcs.cancelAll()

			// Tell the router we are down
			rcs.routerControlChannel <- SocketDownEvent{Sender: rcs, Error: v.Error}

			return

		case CloseCommandMsg:

			// Terminate the outsanding requests
			rcs.cancelAll()

			// Tell the router we are down
			rcs.routerControlChannel <- SocketDownEvent{Sender: rcs}

			return

			// Received message in the UDP Socket. Sent by the readLoop
		case RadiusResponseMsg:

			// Get the data necessary to get from requests map
			endpoint := v.remote.String()
			radiusId := v.packetBytes[1]
			if epReqMap, ok := rcs.requestsMap[endpoint]; !ok {
				instrumentation.PushRadiusClientResponseStalled(endpoint, string(v.packetBytes[0]))
				config.GetLogger().Debugf("unsolicited response from endpoint %s", endpoint)
				continue
			} else if reqCtx, ok := epReqMap[radiusId]; !ok {
				instrumentation.PushRadiusClientResponseStalled(endpoint, string(v.packetBytes[0]))
				config.GetLogger().Debugf("unsolicited response from endpoint %s", endpoint)
				continue
			} else {

				// Check authenticator
				if !radiuscodec.ValidateResponseAuthenticator(v.packetBytes, reqCtx.authenticator, reqCtx.secret) {
					config.GetLogger().Warnf("bad authenticator from %s", endpoint)
					continue
				}

				// Decode the packet
				radiusPacket, err := radiuscodec.RadiusPacketFromBytes(v.packetBytes, reqCtx.secret)
				if err != nil {
					config.GetLogger().Errorf("error decoding packet from %s %s", endpoint, err)
					continue
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
				config.GetLogger().Errorf("could not get an id: %s", error)
				v.rchan <- error
				close(v.rchan)
				continue
			}

			packetBytes, error := v.packet.ToBytes(v.secret, radiusId)
			if error != nil {
				config.GetLogger().Errorf("error marshaling packet: %s", error)
				v.rchan <- error
				close(v.rchan)
				continue
			}

			remoteAddr, _ := net.ResolveUDPAddr("udp", v.endpoint)
			_, err := rcs.socket.WriteTo(packetBytes, remoteAddr)
			if err != nil {
				config.GetLogger().Errorf("error writing packet: %s", error)
				v.rchan <- error
				close(v.rchan)

				// Finalize
				rcs.eventLoopChannel <- CloseCommandMsg{}
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
				secret:        v.secret,
				authenticator: v.packet.Authenticator,
			}

			instrumentation.PushRadiusClientRequest(v.endpoint, string(v.packet.Code))
			config.GetLogger().Debugf("-> Client sent RadiusPacket %s\n", v.packet)

		case CancelRequestMsg:

			if epMap, found := rcs.requestsMap[v.endpoint]; !found {
				config.GetLogger().Debugf("tried to cancel not existing request %s:%d", v.endpoint, v.radiusId)
				continue
			} else if reqCtx, found := epMap[v.radiusId]; !found {
				config.GetLogger().Debugf("tried to cancel not existing request %s:%d", v.endpoint, v.radiusId)
				continue
			} else {
				reqCtx.rchan <- fmt.Errorf("timeout")
				close(reqCtx.rchan)
				delete(rcs.requestsMap[v.endpoint], v.radiusId)
				instrumentation.PushRadiusClientTimeout(v.endpoint, reqCtx.key.Code)
			}
		}
	}
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
				// TODO: Verify that if closed, this is the path
				// Socket was closed
				rcs.eventLoopChannel <- ReadEOFMsg{}
			} else {
				// Unexpected error
				rcs.eventLoopChannel <- ReadErrorMsg{err}
			}
			break
		}

		// Send to eventLoop
		var packetBytes = make([]byte, packetSize)
		copy(packetBytes, reqBuf[:packetSize])

		rcs.eventLoopChannel <- RadiusResponseMsg{
			remote:      *clientAddr.(*net.UDPAddr),
			packetBytes: packetBytes,
		}
	}

	// Signal that we are finished
	close(ch)
}

// Sends a Radius request and gets the answer or error as a message to the specified channel.
// The response channel is closed just after sending the reponse or error
func (rcs *RadiusClientSocket) RadiusExchange(endpoint string, rp *radiuscodec.RadiusPacket, timeout time.Duration, secret string, rc chan interface{}) {
	if cap(rc) < 1 {
		panic("using an unbuffered response channel")
	}

	// Make sure the eventLoop channel is not closed until this finishes
	rcs.wg.Add(1)
	defer rcs.wg.Done()

	code := rp.Code
	if code != radiuscodec.ACCESS_REQUEST && code != radiuscodec.ACCOUNTING_REQUEST && code != radiuscodec.COA_REQUEST && code != radiuscodec.DISCONNECT_REQUEST {
		rc <- fmt.Errorf("code is not for request, but %d", code)
		return
	}

	// Send myself the message
	rcs.eventLoopChannel <- RadiusRequestMsg{
		endpoint: endpoint,
		packet:   rp,
		timeout:  timeout,
		secret:   secret,
		rchan:    rc}
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

// Cancells all outstanding requests
func (rcs *RadiusClientSocket) cancelAll() {
	// TODO: Map is being modified while being iterated
	for ep := range rcs.requestsMap {
		for rid := range rcs.requestsMap[ep] {
			requestContext := rcs.requestsMap[ep][rid]

			// Cancel timer
			if requestContext.timer.Stop() {
				// The after func has not been called
				rcs.wg.Done()
			} else {
				// Drain the channel
				<-requestContext.timer.C
			}
			// Send the error
			requestContext.rchan <- fmt.Errorf("request cancelled due to Socket down")
			close(requestContext.rchan)
			delete(rcs.requestsMap[ep], rid)
		}
		delete(rcs.requestsMap, ep)
	}
}
