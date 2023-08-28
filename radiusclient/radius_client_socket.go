package radiusclient

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/francistor/igor/core"
)

// Sent to the parent RadiusClient when the connection and the eventloop are terminated, due
// to a request or an error. The RadiusClient can then invoke the Close() command
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

// Sent to the eventLoop by the timeout handler function
type CancelRequestMsg struct {
	// To identify the resquest. <ipaddr>:<port> format
	endpoint string

	// To match responses with requests, according to the radius packet format
	radiusId byte

	// Currently only "timeout"
	reason error
}

// Send to the event loop to finalize the object
type SetDownCommandMsg struct {
}

// General error reading from the socket
type ReadErrorMsg struct {
	Error error
}

//////////////////////////////////////////////////////////////////////////////////

// Context data for an in flight request. Stored in the requestMap indexed by
// endpoint and radiusId
type RequestContext struct {

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
// authenticator and timer, in order to match requests with answers.
type RadiusClientSocket struct {

	// The port used
	port int

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

	// Passed as parameter. To report events back to the RadiusClient
	controlChannel chan interface{}

	// Wait group to be used on each goroutine launched, to make sure that
	// the eventloop channel is not used after being closed
	wg sync.WaitGroup

	// StatusTerminated if we should close gracefully
	status int32
}

// Creation function
func NewRadiusClientSocket(controlChannel chan interface{}, bindAddress string, originPort int) *RadiusClientSocket {

	// Bind socket
	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindAddress, originPort))
	if err != nil {
		panic(fmt.Sprintf("could not bind client socket to %s:%d: %s", bindAddress, originPort, err))
	}

	rcs := RadiusClientSocket{
		port:                originPort,
		requestsMap:         make(map[string]map[byte]RequestContext),
		lastRadiusIdMap:     make(map[string]byte),
		eventLoopChannel:    make(chan interface{}, EVENTLOOP_CAPACITY),
		readLoopDoneChannel: make(chan bool, 1),
		controlChannel:      controlChannel,
		socket:              socket,
	}

	go rcs.eventLoop()
	go rcs.readLoop(rcs.readLoopDoneChannel)

	return &rcs
}

// Starts the closing process
func (rcs *RadiusClientSocket) SetDown() {
	core.GetLogger().Debugf("client socket for %s terminating", rcs.socket.LocalAddr().String())

	rcs.eventLoopChannel <- SetDownCommandMsg{}
}

// Closes the event loop channel
// Use this method only after a PeerDown event has been received
// Takes some time to execute
func (rcs *RadiusClientSocket) Close() {

	// Wait for the readLoop to stop
	if rcs.readLoopDoneChannel != nil {
		<-rcs.readLoopDoneChannel
	}

	// Wait until all goroutines and outstanding requests exit
	rcs.wg.Wait()

	// Terminate the event loop
	rcs.eventLoopChannel <- ClientCloseCommand{}

	close(rcs.eventLoopChannel)

	core.GetLogger().Debugf("RadiusClientSocket closed")
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

		case ClientCloseCommand:

			// Terminate the event loop
			return

		case ReadErrorMsg:

			rcs.socket.Close()

			// Terminate the outsanding requests
			rcs.cancelAll()

			// Tell the radiusclient we are down
			rcs.controlChannel <- SocketDownEvent{Sender: rcs, Error: v.Error}

		case SetDownCommandMsg:

			// Set the status
			atomic.StoreInt32(&rcs.status, StatusTerminated)

			rcs.socket.Close()

			// Terminate the outsanding requests
			rcs.cancelAll()

			// Tell the radiusclient we are down
			rcs.controlChannel <- SocketDownEvent{Sender: rcs}

			// Received message in the UDP Socket. Sent by the readLoop
		case RadiusResponseMsg:

			// Get the data necessary to get from requests map
			endpoint := v.remote.String()
			code := string(v.packetBytes[0])
			radiusId := v.packetBytes[1]
			if epReqMap, ok := rcs.requestsMap[endpoint]; !ok {
				core.RecordRadiusClientResponseStalled(endpoint, code)
				core.GetLogger().Debugf("unsolicited or stalled response from endpoint %s", endpoint)
				continue
			} else if requestContext, ok := epReqMap[radiusId]; !ok {
				core.RecordRadiusClientResponseStalled(endpoint, code)
				core.GetLogger().Debugf("unsolicited or stalled response from endpoint %s and id %d", endpoint, radiusId)
				continue
			} else {

				// Cancel timer
				if requestContext.timer.Stop() {
					// The after func has not been called
					rcs.wg.Done()
				} else {
					// Drain the channel. https://itnext.io/go-timer-101252c45166
					select {
					case <-requestContext.timer.C:
					default:
					}
				}

				// Remove from outstanding requests
				delete(epReqMap, radiusId)

				// Check authenticator
				if !core.ValidateResponseAuthenticator(v.packetBytes, requestContext.authenticator, requestContext.secret) {
					core.RecordRadiusClientResponseDrop(endpoint, code)
					core.GetLogger().Warnf("bad authenticator from %s", endpoint)

					// Send the answer to the requester
					requestContext.rchan <- fmt.Errorf("bad authenticator")
					close(requestContext.rchan)
					continue
				}

				// Decode the packet
				radiusPacket, err := core.NewRadiusPacketFromBytes(v.packetBytes, requestContext.secret, requestContext.authenticator)
				if err != nil {
					core.RecordRadiusClientResponseDrop(endpoint, code)
					core.GetLogger().Errorf("error decoding packet from %s %s", endpoint, err)
					// Send the answer to the requester
					requestContext.rchan <- fmt.Errorf("could not decode packet")
					close(requestContext.rchan)
					continue
				}

				core.RecordRadiusClientResponse(endpoint, strconv.Itoa(int(radiusPacket.Code)))
				core.GetLogger().Debugf("<- Client received RadiusPacket %s\n", radiusPacket)

				// Send the answer to the requester
				requestContext.rchan <- radiusPacket
				close(requestContext.rchan)

			}

		case ClientRadiusRequestMsg:

			radiusId, err := rcs.getNextRadiusId(v.endpoint, v.radiusId)
			if err != nil {
				core.GetLogger().Errorf("could not get an id: %s", err)
				v.rchan <- err
				close(v.rchan)

				// Corresponding to the Add(1) in SendRadiusRequest
				rcs.wg.Done()
				continue
			}

			packetBytes, err := v.packet.ToBytes(v.secret, radiusId)
			if err != nil {
				core.GetLogger().Errorf("error marshaling packet: %s", err)
				v.rchan <- err
				close(v.rchan)

				// Corresponding to the Add(1) in SendRadiusRequest
				rcs.wg.Done()
				continue
			}

			remoteAddr, _ := net.ResolveUDPAddr("udp", v.endpoint)
			_, err = rcs.socket.WriteTo(packetBytes, remoteAddr)
			if err != nil {
				core.GetLogger().Errorf("error writing packet or socket closed: %v", err)
				v.rchan <- err
				close(v.rchan)

				// Finalize
				rcs.eventLoopChannel <- SetDownCommandMsg{}

				// Corresponding to the Add(1) in SendRadiusRequest
				rcs.wg.Done()
				continue
			}

			// For the timer to be created below
			rcs.wg.Add(1)

			// Set request map and start timer
			// resquestsMap[v.endpoint] will exist after successful getNextRadiusId
			rcs.requestsMap[v.endpoint][radiusId] = RequestContext{
				rchan: v.rchan,
				timer: time.AfterFunc(v.timeout, func() {
					// This will be called if the timer expires
					defer func() {
						rcs.wg.Done()
					}()
					if v.serverTries <= 1 {
						rcs.eventLoopChannel <- CancelRequestMsg{endpoint: v.endpoint, radiusId: radiusId, reason: fmt.Errorf("timeout")}
					} else {
						retriedClientRadiusRequest := v
						retriedClientRadiusRequest.serverTries--
						retriedClientRadiusRequest.radiusId = radiusId

						rcs.wg.Add(1)
						rcs.eventLoopChannel <- retriedClientRadiusRequest
					}
					core.RecordRadiusClientTimeout(v.endpoint, strconv.Itoa(int(v.packet.Code)))

				}),
				secret: v.secret,
				// The authenticator is generated after ToBytes is called!
				authenticator: v.packet.Authenticator,
			}

			core.RecordRadiusClientRequest(v.endpoint, strconv.Itoa(int(v.packet.Code)))
			core.GetLogger().Debugf("-> Client sent RadiusPacket with Identifier %d - %s\n", radiusId, v.packet)

			// Corresponding to the Add(1) in SendRadiusRequest
			rcs.wg.Done()

		case CancelRequestMsg:

			if epMap, found := rcs.requestsMap[v.endpoint]; !found {
				core.GetLogger().Debugf("tried to cancel not existing request %s:%d", v.endpoint, v.radiusId)
				continue
			} else if reqCtx, found := epMap[v.radiusId]; !found {
				core.GetLogger().Debugf("tried to cancel not existing request %s:%d", v.endpoint, v.radiusId)
				continue
			} else {
				reqCtx.rchan <- fmt.Errorf("timeout")
				close(reqCtx.rchan)
				delete(rcs.requestsMap[v.endpoint], v.radiusId)
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
			if atomic.LoadInt32(&rcs.status) != StatusTerminated {
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
func (rcs *RadiusClientSocket) SendRadiusRequest(request ClientRadiusRequestMsg) {
	if cap(request.rchan) < 1 {
		panic("using an unbuffered response channel")
	}

	code := request.packet.Code
	if code != core.ACCESS_REQUEST && code != core.ACCOUNTING_REQUEST && code != core.COA_REQUEST && code != core.DISCONNECT_REQUEST {
		request.rchan <- fmt.Errorf("code is not for request, but %d", code)
		close(request.rchan)
		return
	}

	// Make sure the message is processed
	rcs.wg.Add(1)
	// Send myself the message
	rcs.eventLoopChannel <- request
}

// Gets the next radiusid to use, or error if all are busy
// Allocates the nested map if not already instantiated
// After calling this function it can be ensured that the requestsMap contains
// a map for the specifed endpoint
// Id 0 is special and never allocated. If currentRadiusId is not 0, returns the same
func (rcs *RadiusClientSocket) getNextRadiusId(endpoint string, currentRadiusId byte) (byte, error) {

	// Do nothing if already have a radiusId
	if currentRadiusId != 0 {
		return currentRadiusId, nil
	}

	idMap, found := rcs.requestsMap[endpoint]
	if !found {
		// Map for this endpoint does not exist yet. Create it
		rcs.requestsMap[endpoint] = make(map[byte]RequestContext)
	}

	lastId, found := rcs.lastRadiusIdMap[endpoint]
	if !found {
		// Map for this endpoint does not exist yet. Create it
		rcs.lastRadiusIdMap[endpoint] = 1
	}

	// Try to get a radius Id
	nextId := lastId
	for i := 0; i < 255; i++ {
		if nextId == 255 {
			nextId = 1
		} else {
			nextId = nextId + 1
		}
		if _, ok := idMap[nextId]; !ok {
			rcs.lastRadiusIdMap[endpoint] = nextId
			if nextId == 0 {
				panic("Radius id should never be 0")
			}
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
				// Drain the channel. https://itnext.io/go-timer-101252c45166
				select {
				case <-requestContext.timer.C:
				default:
				}
			}
			// Send the error
			requestContext.rchan <- fmt.Errorf("request cancelled due to Socket down")
			close(requestContext.rchan)
			delete(rcs.requestsMap[ep], rid)
		}
		delete(rcs.requestsMap, ep)
	}
}

// For testing purposes only
func (rcs *RadiusClientSocket) closeSocket() {
	rcs.socket.Close()
}
