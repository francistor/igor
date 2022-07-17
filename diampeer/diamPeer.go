package diampeer

import (
	"bufio"
	"context"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/instrumentation"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	StatusConnecting  = 1
	StatusConnected   = 2
	StatusEngaged     = 3
	StatusTerminating = 4 // No more requests allowed
	StatusTerminated  = 5 // EventLoop not running
)

const (
	EVENTLOOP_CAPACITY = 100
)

// Ouput Events (control channel)

// Sent to the Router, via the output channel passed as parameter, to signal
// that the Peer object is down and should be recycled
// If the reason is an error (e.g. bad response from the other, communication problem),
// etc. the Error field will be not null
type PeerDownEvent struct {
	// Myself
	Sender *DiameterPeer
	// Will be nil if the reason is not an error
	Error error
}

// Sent to the Router, via the output channel passed as parameter, to signal
// that the Peer object is ready to be used, that is, after the CER/CEA has been
// completed. If the Peer is passive, the DiameterHost attribute will be non nil
// and set as the reported DiameterHost.
// The Router should check that there is no other Peer for the same DiameterHost,
// otherwise closing this peer
type PeerUpEvent struct {
	// Myself
	Sender *DiameterPeer
	// Reported identity of the remote peer
	DiameterHost string
}

// Internal messages

// Internal message sent to myself when the CER/CEA has completed successfully
type PeerUpMsg struct {
	// Reported identity of the remote peer
	DiameterHost string
}

// Message from me to a Diameteer Peer. May be a Request or an Answer
// If a request of non base diameter application, RChan will contain
// the channel on which the answer must be written
type EgressDiameterMsg struct {
	Message *diamcodec.DiameterMessage

	// nil if a Response or base application
	RChan chan interface{}

	// Timeout to set
	timeout time.Duration
}

// Message received from a Diameter Peer. May be a Request or an Answer
// Sent by the readLoop to the eventLoop
type IngressDiameterMsg struct {
	Message *diamcodec.DiameterMessage
}

// Timeout expired waiting for a Diameter Answer or any other cancellation reason
// The HopByHopId will hold the key in the requestsMap
type CancelRequestMsg struct {
	HopByHopId uint32
	Reason     error
}

// Send internally to force a disconnection, moving the Peer to
// the closed state
type PeerCloseCommandMsg struct {
}

// Sent when the connecton with the peer is successful (Active Peer)
// The Peer will move to the connected status and will start the
// CER/CEA handshake
type ConnectionEstablishedMsg struct {
	Connection net.Conn
}

// Sent then the connection with the peer fails (Active Peer)
// The peer will report a down status to be recycled
type ConnectionErrorMsg struct {
	Error error
}

// Sent when the connection with the remote peer reports EOF
// The peer will report a down status to be recycled
type ReadEOFMsg struct{}

// Sent when the connection with the remote peer reports a reading error
// The peer will report a down status to be recycled
type ReadErrorMsg struct {
	Error error
}

// Sent when the connection with the remote peer reports a write error
// The peer will report a down status to be recycled
type WriteErrorMsg struct {
	Error error
}

// Sent periodically for device watchdog implementation
type WatchdogMsg struct {
}

/////////////////////////////////////////////

// Type for functions that handle the diameter requests received
// If an error is returned, no diameter answer is sent. Implementers should always generate a diameter answer instead
type MessageHandler func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error)

// Context data for an in flight request
type RequestContext struct {

	// Metric key. Used because the message will not be available in a timeout
	Key instrumentation.PeerDiameterMetricKey

	// Channel on which the answer will be sent
	RChan chan interface{}

	// Timer
	Timer *time.Timer
}

// This object abstracts the operations against a Diameter Peer
// It implements the Actor model: all internal variables are modified
// from an internal single threaded EventLoop and message passing

// A DiameterPeer is created using one of the NewXXX methods, passing a control channel back
// to the Router. A PeerDown will eventually be sent, either because the Peer engaging process
// did not terminate correctly, because an error reading or writting from the TCP socket happens,
// or due to explicit termination (Disengage method). The DiameterPeer object is then set to
// "Closed" state, but the Close() method must be called explicitly to close the internal channel
// for the event loop. After the engagement process terminates correctly, the PeerUp event is sent
// through the control channel

type DiameterPeer struct {

	// Holds the configuration instance for this DiameterPeer
	ci *config.PolicyConfigurationManager

	// Holds the Peer configuration
	// Passed during instantiation if Peer is Active
	// Filled after CER/CEA exchange if Peer is Passive
	PeerConfig config.DiameterPeer

	// Input and output channels

	// Created iternally. This is for the Actor model loop
	eventLoopChannel chan interface{}

	// Created internaly, for synchronizing the event and read loops
	// The ReadLoop will send a message when exiting, signalling that
	// it will not send more messages to the eventLoopChannel, so it
	// can be closed as far as the ReadLoop is concerned
	readLoopDoneChannel chan bool

	// Passed as parameter. To report events back to the Router
	routerControlChannel chan interface{}

	// The Status of the object (one of the const defined above)
	status int

	// Internal
	connection net.Conn
	connReader *bufio.Reader
	connWriter *bufio.Writer

	// Canceller of TCP connection with Peer
	cancel context.CancelFunc

	// Outstanding requests map
	// Maps HopByHopIds to a channel where the response or a timeout will be sent
	requestsMap map[uint32]RequestContext

	// Registered Handler for incoming messages
	handler MessageHandler

	// Ticker for watchdog requests
	watchdogTicker *time.Ticker

	// Number of unanswered watchdog requests
	outstandingDWA int

	// Wait group to be used on each goroutine launched, to make sure that
	// the eventloop channel is not used after being closed
	wg sync.WaitGroup
}

// Creates a new DiameterPeer when we are expected to establish the connection with the other side
// and initiate the CER/CEA handshake
func NewActiveDiameterPeer(configInstanceName string, rc chan interface{}, peer config.DiameterPeer, handler MessageHandler) *DiameterPeer {

	// Create the Peer struct
	dp := DiameterPeer{
		ci:                   config.GetPolicyConfigInstance(configInstanceName),
		eventLoopChannel:     make(chan interface{}, EVENTLOOP_CAPACITY),
		routerControlChannel: rc,
		PeerConfig:           peer,
		requestsMap:          make(map[uint32]RequestContext),
		handler:              handler,
	}

	config.GetLogger().Debugf("creating active diameter peer for %s", peer.DiameterHost)

	dp.status = StatusConnecting

	// Default value for timeout
	timeout := peer.ConnectionTimeoutMillis
	if timeout == 0 {
		timeout = 5000
	}

	// Do not close until the connecton thread finishes. Wait for this wg is in the Close() method
	dp.wg.Add(1)

	// This will eventually send a ConnectionEstablishedMsg or ConnectionErrorMsg
	go dp.connect(timeout, peer.IPAddress, peer.Port)

	// Start the event loop
	go dp.eventLoop()

	return &dp
}

// Creates a new DiameterPeer when the connection has been alread accepted
func NewPassiveDiameterPeer(configInstanceName string, rc chan interface{}, conn net.Conn, handler MessageHandler) *DiameterPeer {

	// Create the Peer Struct
	dp := DiameterPeer{
		ci:                   config.GetPolicyConfigInstance(configInstanceName),
		eventLoopChannel:     make(chan interface{}, EVENTLOOP_CAPACITY),
		routerControlChannel: rc,
		connection:           conn,
		requestsMap:          make(map[uint32]RequestContext),
		handler:              handler}

	config.GetLogger().Debugf("creating passive diameter peer for %s", conn.RemoteAddr().String())

	dp.status = StatusConnected

	dp.connReader = bufio.NewReader(dp.connection)
	dp.connWriter = bufio.NewWriter(dp.connection)

	dp.readLoopDoneChannel = make(chan bool, 1)
	go dp.readLoop(dp.readLoopDoneChannel)

	go dp.eventLoop()

	return &dp
}

// Terminates the Peer connection and the event loop
// A PeerDown message will be sent through the control channel
// after which the Close() command may be invoked
func (dp *DiameterPeer) SetDown() {
	dp.eventLoopChannel <- PeerCloseCommandMsg{}

	config.GetLogger().Debugf("%s terminating", dp.PeerConfig.DiameterHost)
}

// Closes the event loop channel
// Use this method only after a PeerDown event has been received
// Takes some time to execute
func (dp *DiameterPeer) Close() {

	// Wait for the readLoop to stop
	if dp.readLoopDoneChannel != nil {
		<-dp.readLoopDoneChannel
	}

	// Wait until all goroutines exit
	dp.wg.Wait()

	close(dp.eventLoopChannel)

	config.GetLogger().Debugf("%s closed", dp.PeerConfig.DiameterHost)
}

// Event Loop
func (dp *DiameterPeer) eventLoop() {

	defer func() {
		// Cancel ticker for watchdog message
		if dp.watchdogTicker != nil {
			dp.watchdogTicker.Stop()
		}

		// Close the connection (another time, should not make harm)
		if dp.connection != nil {
			dp.connection.Close()
		}

	}()

	// Initialize to something, in order to be able to select below.
	// A proper time is set when the status becomes "Engaged"
	dp.watchdogTicker = time.NewTicker(time.Duration(999999) * time.Hour)

	for {
		select {

		case <-dp.watchdogTicker.C:
			if dp.status == StatusEngaged {
				dp.eventLoopChannel <- WatchdogMsg{}
			}

		case in := <-dp.eventLoopChannel:

			switch v := in.(type) {

			// Connect goroutine reports connection established
			// Start the event loop and CER/CEA handshake
			case ConnectionEstablishedMsg:

				config.GetLogger().Debugf("connection established with %s", v.Connection.RemoteAddr().String)

				dp.connection = v.Connection
				dp.connReader = bufio.NewReader(dp.connection)
				dp.connWriter = bufio.NewWriter(dp.connection)

				// Start the read loop
				dp.readLoopDoneChannel = make(chan bool, 1)
				go dp.readLoop(dp.readLoopDoneChannel)

				dp.status = StatusConnected

				// Active Peer. We'll send the CER
				cer, err := diamcodec.NewDiameterRequest("Base", "Capabilities-Exchange")
				cer.AddOriginAVPs(dp.ci)
				if err != nil {
					panic("could not create a CER")
				}
				// Finish building the CER message
				dp.pushCEAttributes(cer)

				// Send the message to the peer
				dp.eventLoopChannel <- EgressDiameterMsg{Message: cer}

			// Connect goroutine reports connection could not be established
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ConnectionErrorMsg:

				config.GetLogger().Errorf("connection error %s", v.Error)
				dp.status = StatusTerminated
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
				return

			// readLoop goroutine reports the connection is closed
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ReadEOFMsg:

				if dp.status < StatusTerminating {
					config.GetLogger().Debugf("connection terminated by remote peer %s", dp.connection.RemoteAddr().String())
				} else {
					config.GetLogger().Errorf("connection terminated with remote peer %s", dp.connection.RemoteAddr().String())
				}

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: nil}
				return

			// readLoop goroutine reports a read error
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ReadErrorMsg:

				if dp.status < StatusTerminating {
					config.GetLogger().Errorf("connection read error %v with remote peer %s", v.Error, dp.connection.RemoteAddr().String())
				} else {
					config.GetLogger().Debugf("connection terminating with remote peer %s. Last error %v", dp.connection.RemoteAddr().String(), v.Error)
				}

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated

				// Tell the router we are down
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: v.Error}

				return

			// Same for writes
			case WriteErrorMsg:

				config.GetLogger().Errorf("write error %s with remote peer %s", v.Error, dp.connection.RemoteAddr().String)

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated

				// Tell the router we are down
				dp.eventLoopChannel <- PeerDownEvent{Sender: dp, Error: v.Error}

				return

			case PeerUpMsg:
				dp.status = StatusEngaged

				// Tell the Router we are up
				dp.routerControlChannel <- PeerUpEvent{Sender: dp, DiameterHost: v.DiameterHost}

				// Reinitialize the timer with the right duration
				dp.watchdogTicker.Stop()
				dp.watchdogTicker = time.NewTicker(time.Duration(dp.PeerConfig.WatchdogIntervalMillis) * time.Millisecond)

			// Initiate closing procedure
			case PeerCloseCommandMsg:

				config.GetLogger().Debug("processing PeerCloseCommandMsg")

				dp.status = StatusTerminated

				// In case it was still connecting
				if dp.cancel != nil {
					dp.cancel()
				}

				// Close the connection. Any reads will return with error in the read loop, which will terminate
				// and send control message through the readloopChannel
				if dp.connection != nil {
					dp.connection.Close()
				}

				// Cancellation of all outstanding requests
				for hopId := range dp.requestsMap {
					config.GetLogger().Debugf("cancelling request %d", hopId)
					requestContext := dp.requestsMap[hopId]

					// Cancel timer
					if requestContext.Timer.Stop() {
						// The after func has not been called
						dp.wg.Done()
					} else {
						// Drain the channel
						<-requestContext.Timer.C
					}
					// Send the error
					requestContext.RChan <- fmt.Errorf("request cancelled due to Peer down")
					close(requestContext.RChan)
					delete(dp.requestsMap, hopId)
				}

				// Tell the Router we are finished
				dp.routerControlChannel <- PeerDownEvent{Sender: dp}

				return

				// Send a message to the peer. May be a request or an answer
			case EgressDiameterMsg:

				if dp.status == StatusConnected || dp.status == StatusEngaged {

					// Check not duplicate
					hbhId := v.Message.HopByHopId
					if _, ok := dp.requestsMap[hbhId]; ok && v.RChan != nil {
						v.RChan <- fmt.Errorf("duplicated HopByHopId")
						break
					}

					config.GetLogger().Debugf("-> Sending Message %s\n", v.Message)
					_, err := v.Message.WriteTo(dp.connection)
					if err != nil {
						// There was an error writing. Will close the connection
						if dp.status < StatusTerminating {
							dp.eventLoopChannel <- WriteErrorMsg{err}
							dp.status = StatusTerminating
						}

						// Signal the error in the response channel for the input request
						// Do all necessary things to cancell the request
						if v.Message.IsRequest && v.RChan != nil {
							v.RChan <- err
						}

						// No statistics, because the Peer will die

						break
					}

					// All good.
					// If it was a Request, store in the outstanding request map
					// RChan may be nil if it is a base application message
					if v.Message.IsRequest {
						instrumentation.PushPeerDiameterRequestSent(dp.PeerConfig.DiameterHost, v.Message)
						if v.RChan != nil {
							// Set timer
							dp.wg.Add(1)
							timer := time.AfterFunc(v.timeout, func() {
								// This will be called if the timer expires
								dp.eventLoopChannel <- CancelRequestMsg{HopByHopId: v.Message.HopByHopId, Reason: fmt.Errorf("Timeout")}
								defer dp.wg.Done()
							})

							dp.requestsMap[v.Message.HopByHopId] = RequestContext{RChan: v.RChan, Timer: timer, Key: instrumentation.PeerDiameterMetricFromMessage(dp.PeerConfig.DiameterHost, v.Message)}
						}
					} else {
						instrumentation.PushPeerDiameterAnswerSent(dp.PeerConfig.DiameterHost, v.Message)
					}

				} else {
					config.GetLogger().Errorf("%s %s message was not sent because status is %d", v.Message.ApplicationName, v.Message.CommandName, dp.status)
				}

				// Received message from peer
			case IngressDiameterMsg:

				config.GetLogger().Debugf("<- Receiving Message %s\n", v.Message)

				if v.Message.IsRequest {

					instrumentation.PushPeerDiameterRequestReceived(dp.PeerConfig.DiameterHost, v.Message)

					// Check if it is a Base application message (code for Base application is 0)
					if v.Message.ApplicationId == 0 {
						switch v.Message.CommandName {

						case "Capabilities-Exchange":
							if originHost, err := dp.handleCER(v.Message); err != nil {
								// There was an error
								// dp.status = StatusTerminating
								dp.eventLoopChannel <- PeerCloseCommandMsg{}
							} else {
								// The router must check that there is no other connection for the same peer
								// and set state to active
								dp.status = StatusEngaged
								dp.eventLoopChannel <- PeerUpMsg{DiameterHost: originHost}
							}

						case "Device-Watchdog":
							dwa := diamcodec.NewDiameterAnswer(v.Message)
							dwa.AddOriginAVPs(dp.ci)
							dwa.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
							dp.eventLoopChannel <- EgressDiameterMsg{Message: dwa}

						case "Disconnect-Peer":
							dpa := diamcodec.NewDiameterAnswer(v.Message)
							dpa.AddOriginAVPs(dp.ci)
							dp.eventLoopChannel <- EgressDiameterMsg{Message: dpa}
							dp.eventLoopChannel <- PeerCloseCommandMsg{}
							dp.status = StatusTerminating

						default:
							config.GetLogger().Warnf("command %d for base applicaton not found in dictionary", v.Message.CommandCode)
						}

					} else {
						// Reveived a non base request. Invoke handler
						// Make sure the eventLoopChannel is not closed until the response is received
						dp.wg.Add(1)
						go func() {
							defer dp.wg.Done()
							resp, err := dp.handler(v.Message)
							if err != nil {
								config.GetLogger().Error(err)
								// Send an error UNABLE_TO_COMPLY
								errorResp := diamcodec.NewDiameterAnswer(v.Message)
								errorResp.AddOriginAVPs(dp.ci)
								errorResp.Add("Result-Code", diamcodec.DIAMETER_UNABLE_TO_COMPLY)
								dp.eventLoopChannel <- EgressDiameterMsg{Message: errorResp}
							} else {
								dp.eventLoopChannel <- EgressDiameterMsg{Message: resp}
							}
						}()
					}
				} else {
					// Received an answer
					instrumentation.PushPeerDiameterAnswerReceived(dp.PeerConfig.DiameterHost, v.Message)

					if v.Message.ApplicationId == 0 {
						// Base answer
						switch v.Message.CommandName {
						case "Capabilities-Exchange":
							doDisconnect := true
							// Received capabilities exchange answer
							originHostAVP, err := v.Message.GetAVP("Origin-Host")
							if err != nil {
								config.GetLogger().Errorf("error getting Origin-Host %s", err)
							} else if originHostAVP.GetString() != dp.PeerConfig.DiameterHost {
								config.GetLogger().Errorf("error in CER. Got origin host %s instead of %s", originHostAVP.GetString(), dp.PeerConfig.DiameterHost)
							} else if v.Message.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
								config.GetLogger().Errorf("error in CER. Got Result code %d", v.Message.GetResultCode())
							} else {
								// All good.
								doDisconnect = false
							}

							if doDisconnect {
								dp.status = StatusTerminating
								dp.eventLoopChannel <- PeerCloseCommandMsg{}
							} else {
								dp.eventLoopChannel <- PeerUpMsg{DiameterHost: dp.PeerConfig.DiameterHost}
							}

						case "Device-Watchdog":
							config.GetLogger().Debug("received dwa")
							if v.Message.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
								config.GetLogger().Errorf("bad result code in answer to DWR: %d", v.Message.GetResultCode())
								dp.eventLoopChannel <- PeerCloseCommandMsg{}
								dp.status = StatusTerminating
							} else {
								dp.outstandingDWA--
							}
						default:
							config.GetLogger().Warnf("command %d for base applicaton not found in dictionary", v.Message.CommandCode)
						}
					} else {
						// Non base answer
						if requestContext, ok := dp.requestsMap[v.Message.HopByHopId]; !ok {
							instrumentation.PushPeerDiameterAnswerStalled(dp.PeerConfig.DiameterHost, v.Message)
							config.GetLogger().Errorf("stalled diameter answer: '%v'", *v.Message)
						} else {
							// Cancel timer
							if requestContext.Timer.Stop() {
								// The after func has not been called
								dp.wg.Done()
							} else {
								// Drain the channel
								<-requestContext.Timer.C
							}
							// Send the response
							requestContext.RChan <- v.Message
							close(requestContext.RChan)
							delete(dp.requestsMap, v.Message.HopByHopId)
						}
					}
				}

			case CancelRequestMsg:
				config.GetLogger().Debugf("Cancelling HopByHopId: <%d>\n", v.HopByHopId)
				requestContext, ok := dp.requestsMap[v.HopByHopId]
				if !ok {
					config.GetLogger().Errorf("attempt to cancel an non existing request with HopByHopId %d", v.HopByHopId)
				} else {
					// Send the response
					requestContext.RChan <- v.Reason
					// No more messages will be sent through this channel
					close(requestContext.RChan)
					// Delete the requestmap entry
					delete(dp.requestsMap, v.HopByHopId)
					// Update metric
					instrumentation.PushPeerDiameterRequestTimeout(dp.PeerConfig.DiameterHost, requestContext.Key)
				}

			case WatchdogMsg:
				maxOustandingDWA := 2
				config.GetLogger().Debugf("dwr tick")

				// Here we do the checking of the DWA that are pending
				if dp.outstandingDWA > maxOustandingDWA {
					config.GetLogger().Errorf("too many unanswered DWR: %d", maxOustandingDWA)
					dp.eventLoopChannel <- PeerCloseCommandMsg{}
				}

				// Create request
				dwr, err := diamcodec.NewDiameterRequest("Base", "Device-Watchdog")
				dwr.AddOriginAVPs(dp.ci)
				if err != nil {
					panic("could not create a DWR")
				}
				dp.eventLoopChannel <- EgressDiameterMsg{Message: dwr}
				dp.outstandingDWA++
			}
		}
	}

}

// Establishes the connection with the peer
// To be executed in a goroutine
// Should not touch inner variables
func (dp *DiameterPeer) connect(connTimeoutMillis int, ipAddress string, port int) {

	// Create a cancellable deadline
	context, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(connTimeoutMillis)*time.Millisecond))
	dp.cancel = cancel
	defer func() {
		dp.cancel()
		dp.wg.Done()
	}()

	// Connect
	var dialer net.Dialer
	conn, err := dialer.DialContext(context, "tcp4", fmt.Sprintf("%s:%d", ipAddress, port))

	if err != nil {
		dp.eventLoopChannel <- ConnectionErrorMsg{err}
	} else {
		dp.eventLoopChannel <- ConnectionEstablishedMsg{conn}
	}

}

// Reader of peer messages
// To be executed in a goroutine
// Should not touch inner variables
func (dp *DiameterPeer) readLoop(ch chan bool) {
	for {
		// Read a Diameter message from the connection
		dm := diamcodec.DiameterMessage{}
		_, err := dm.ReadFrom(dp.connection)
		if err != nil {
			if err == io.EOF {
				// The remote peer closed
				dp.eventLoopChannel <- ReadEOFMsg{}
			} else {
				// May have closed the connection myself (status will be "StatusTerminating") or be a true error
				dp.eventLoopChannel <- ReadErrorMsg{err}
			}
			break
		} else {
			// Send myself the received message
			dp.eventLoopChannel <- IngressDiameterMsg{Message: &dm}
		}
	}

	// Signal that we are finished
	close(ch)
}

// Sends a Diameter request and gets the answer or error as a message to the specified channel.
// The response channel is closed just after sending the reponse or error
func (dp *DiameterPeer) DiameterExchangeWithChannel(dm *diamcodec.DiameterMessage, timeout time.Duration, rc chan interface{}) {

	if cap(rc) < 1 {
		panic("using an unbuffered response channel")
	}

	// Make sure the eventLoop channel is not closed until this finishes
	dp.wg.Add(1)
	defer dp.wg.Done()

	// Validations
	if dm.ApplicationId == 0 {
		rc <- fmt.Errorf("should not use this method to send a Base Application message")
		return
	}
	if dp.status != StatusEngaged {
		rc <- fmt.Errorf("tried to send a diameter request in a non engaged DiameterPeer. Status is %d", dp.status)
		return
	}
	if !(*dm).IsRequest {
		rc <- fmt.Errorf("diameter message is not a request")
		return
	}

	// Send myself the message
	dp.eventLoopChannel <- EgressDiameterMsg{Message: dm, RChan: rc, timeout: timeout}
}

// Sends a Diameter request and gets the answer or an error (timeout or network error)
func (dp *DiameterPeer) DiameterExhangeWithAnswer(dm *diamcodec.DiameterMessage, timeout time.Duration) (resp *diamcodec.DiameterMessage, e error) {

	// This channel will receive the response
	// It will be closed in the event loop, at the same time as deleting the requestMap entry
	// Use buffered channel, to avoid deadlocks (the DiamPeer writing to a channel when there is nobody listening yet)
	var responseChannel = make(chan interface{}, 1)

	dp.DiameterExchangeWithChannel(dm, timeout, responseChannel)

	r := <-responseChannel
	switch v := r.(type) {
	case error:
		return nil, v
	case *diamcodec.DiameterMessage:
		return v, nil
	}

	panic("unreachable code in DiameterExchangeWithResponse")
}

// Sends the message and executes the handler function when the answer is received
// In case of error, the response will be nill and e will be non nil
func (dp *DiameterPeer) DiameterRequestWithAnswerAsync(dm *diamcodec.DiameterMessage, timeout time.Duration, handler func(resp *diamcodec.DiameterMessage, e error)) {
	go func() {
		handler(dp.DiameterExhangeWithAnswer(dm, timeout))
	}()
}

// Handle received CER message
// May send an error response to the remote peer
// This is executed in the eventLoop
func (dp *DiameterPeer) handleCER(request *diamcodec.DiameterMessage) (string, error) {

	if dp.status != StatusConnected {
		return "", fmt.Errorf("received CER when status in not connected, but %d", dp.status)
	}

	// Depending on the error, we need to reply back with a message or just disconnect
	sendErrorMessage := false

	// Check at least that the peer exists and the origin IP address is valMid
	originHostAVP, err := request.GetAVP("Origin-Host")
	if err == nil {
		originHost := originHostAVP.GetString()

		remoteAddr, _, _ := net.SplitHostPort(dp.connection.RemoteAddr().String())
		remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)

		peersConf := dp.ci.PeersConf()
		if peersConf.ValidateIncomingAddress(originHost, remoteIPAddr.IP) {

			if peerConfig, err := peersConf.FindPeer(originHost); err == nil {
				// Grab the peer configuration
				dp.PeerConfig = peerConfig

				cea := diamcodec.NewDiameterAnswer(request)
				cea.AddOriginAVPs(dp.ci)
				cea.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
				dp.pushCEAttributes(cea)
				dp.eventLoopChannel <- EgressDiameterMsg{Message: cea}

				// All good returns here
				return originHost, nil
			} else {
				config.GetLogger().Errorf("Origin-Host not found in configuration %s while handling CER", originHost)
				sendErrorMessage = true
			}
		} else {
			config.GetLogger().Errorf("invalid diameter peer %s with address %s while handling CER", originHost, remoteIPAddr.IP)
			sendErrorMessage = true
		}
	} else {
		config.GetLogger().Errorf("error getting Origin-Host %s while handling CER", err)
	}

	if sendErrorMessage {
		// Send error message before disconnecting
		cea := diamcodec.NewDiameterAnswer(request)
		cea.AddOriginAVPs(dp.ci)
		cea.Add("Result-Code", diamcodec.DIAMETER_UNKNOWN_PEER)
		dp.eventLoopChannel <- EgressDiameterMsg{Message: cea}
	}

	return "", fmt.Errorf("bad CEA")
}

// Helper function to build CER/CEA
func (dp *DiameterPeer) pushCEAttributes(cer *diamcodec.DiameterMessage) {
	serverConf := dp.ci.DiameterServerConf()

	if serverConf.BindAddress != "0.0.0.0" {
		cer.Add("Host-IP-Address", serverConf.BindAddress)
	}
	cer.Add("Vendor-Id", serverConf.VendorId)
	cer.Add("Product-Name", "igor")
	cer.Add("Firmware-Revision", serverConf.FirmwareRevision)
	// TODO: This number should increase on every restart
	cer.Add("Origin-State-Id", 1)
	// Add supported applications
	routingRules := dp.ci.RoutingRulesConf()
	var relaySet = false
	for _, rule := range routingRules {
		if rule.ApplicationId != "*" {
			if appDict, ok := config.GetDDict().AppByName[rule.ApplicationId]; ok {
				if strings.Contains(appDict.AppType, "auth") {
					cer.Add("Auth-Application-Id", appDict.Code)
				} else if strings.Contains(appDict.AppType, "acct") {
					cer.Add("Acct-Application-Id,", appDict.Code)
				}
			}
		} else {
			if !relaySet {
				cer.Add("Auth-Application-Id", "Relay")
				cer.Add("Acct-Application-Id", "Relay")
				relaySet = true
			}
		}
	}
}
