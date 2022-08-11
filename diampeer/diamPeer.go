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
	StatusConnecting = 1
	StatusConnected  = 2
	StatusEngaged    = 3
	StatusTerminated = 4 // In the process of shuting down
)

const (
	EVENTLOOP_CAPACITY               = 100
	MAX_UNANSWERED_WATCHDOG_REQUESTS = 2
)

// Ouput Events (control channel)

// Sent to the Router, via the output channel passed as parameter, to signal
// that the Peer object is down and should be recycled. If the reason is an
// error (e.g. bad response from the other, communication problem), etc. the Error field
// will be not null.
// Upon sending this event, the eventloop is finished, but some goroutines may be
// still being executed and the TCP socket may still be in use.
// The Router must call Close() on this object to wait for full finalization
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

//////////////////////////////////////////////////////////////////////////////
// Eventloop messages
//////////////////////////////////////////////////////////////////////////////

// Internal message sent to myself when the CER/CEA has completed successfully
type PeerUpMsg struct {
	// Reported identity of the remote peer
	diameterHost string
}

// Message from me to a Diameteer Peer. May be a Request or an Answer
// If a request of non base diameter application, RChan will contain
// the channel on which the answer must be written. Otherwise it will be nil
type EgressDiameterMsg struct {
	message *diamcodec.DiameterMessage

	// nil if a Response or base application
	rchan chan interface{}

	// Timeout to set
	timeout time.Duration
}

// Message received from a Diameter Peer. May be a Request or an Answer
// Sent by the readLoop to the eventLoop
type IngressDiameterMsg struct {
	message *diamcodec.DiameterMessage
}

// Timeout expired waiting for a Diameter Answer or any other cancellation reason
// The HopByHopId will hold the key in the requestsMap
type CancelRequestMsg struct {
	hopByHopId uint32
	// Currently, only timeout is reported
	reason error
}

// Send internally to force a disconnection, moving the Peer to
// the terminated state
type PeerSetDownCommandMsg struct {
	err error
}

// Sent when the connecton with the peer is successful (Active Peer)
// The Peer will move to the connected status and will start the
// CER/CEA handshake
type ConnectionEstablishedMsg struct {
	connection net.Conn
}

// Sent then the connection with the peer fails (Active Peer)
// The peer will report a down status to be recycled
type ConnectionErrorMsg struct {
	err error
}

// Sent when the connection with the remote peer reports EOF
// The peer will report a down status to be recycled
type ReadEOFMsg struct{}

// Sent when the connection with the remote peer reports a reading error
// The peer will report a down status to be recycled
type ReadErrorMsg struct {
	err error
}

// Sent when the connection with the remote peer reports a write error
// The peer will report a down status to be recycled
type WriteErrorMsg struct {
	err error
}

// Sent periodically for device watchdog implementation
type WatchdogMsg struct {
}

/////////////////////////////////////////////

// Type for functions that handle the diameter requests received
// If an error is returned, no diameter answer is sent. Implementers should better generate a diameter answer instead
// with error code
type MessageHandler func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error)

// Context data for an in flight request
type RequestContext struct {

	// Metric key. Used because the message will not be available in a timeout
	key instrumentation.PeerDiameterMetricKey

	// Channel on which the answer or an error will be reported back
	rchan chan interface{}

	// Timer
	timer *time.Timer
}

// This object abstracts the operations against a Diameter Peer
// It implements the Actor model: all internal variables are modified
// from an internal single threaded EventLoop and message passing

// A DiameterPeer is created using one of the NewXXX methods, passing a control channel back
// to the Router. A PeerDown will eventually be sent, either because the Peer engaging process
// did not terminate correctly, because an error reading or writting from the TCP socket happens,
// or due to explicit termination (Disengage method). The DiameterPeer object is then set to
// "Terminated" state, but the Close() method must be called explicitly to close the internal channel
// for the event loop and wait for goroutines to finalize. After the engagement process terminates
// correctly, the PeerUp event is sent through the control channel

type DiameterPeer struct {

	// Holds the Peer configuration
	// Passed during instantiation if Peer is Active
	// Filled after CER/CEA exchange if Peer is Passive
	peerConfig config.DiameterPeer

	// Holds the configuration instance for this DiameterPeer
	ci *config.PolicyConfigurationManager

	// Input and output channels

	// Created iternally. This is for the Actor model loop
	eventLoopChannel chan interface{}

	// Created internaly, for synchronizing the event and read loops
	// The readLoop will send a message when exiting, signalling that
	// it will not send more messages to the eventLoopChannel, so it
	// can be closed as far as the readLoop is concerned
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

	// Number of currently unanswered watchdog requests
	outstandingDWA int

	// Wait group to be used on each goroutine launched, to be waited on Close(),
	// to make sure that the eventloop channel is not used after being closed
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
		peerConfig:           peer,
		requestsMap:          make(map[uint32]RequestContext),
		handler:              handler,
	}

	config.GetLogger().Debugf("creating active diameter peer for %s", peer.DiameterHost)

	dp.status = StatusConnecting

	// Default value for timeout
	timeoutMillis := peer.ConnectionTimeoutMillis
	if timeoutMillis == 0 {
		timeoutMillis = 5000
	}

	// Do not close until the connecton thread finishes. Wait for this wg is in the Close() method
	dp.wg.Add(1)

	// This will eventually send a ConnectionEstablishedMsg or ConnectionErrorMsg
	go dp.connect(time.Duration(timeoutMillis)*time.Millisecond, peer.IPAddress, peer.Port)

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
	dp.eventLoopChannel <- PeerSetDownCommandMsg{}

	config.GetLogger().Debugf("%s terminating", dp.peerConfig.DiameterHost)
}

// Closes the event loop channel
// Use this method only after a PeerDown event has been received
// Takes some time to execute
func (dp *DiameterPeer) Close() {

	// Wait for the readLoop to stop
	if dp.readLoopDoneChannel != nil {
		<-dp.readLoopDoneChannel
	}

	// Wait until all goroutines exit, including timers in outstanding requests
	dp.wg.Wait()

	close(dp.eventLoopChannel)

	config.GetLogger().Debugf("%s closed", dp.peerConfig.DiameterHost)
}

// To hide the internal variable for DiameterPeer configuration
func (dp *DiameterPeer) GetPeerConfig() config.DiameterPeer {
	return dp.peerConfig
}

// Event Loop
func (dp *DiameterPeer) eventLoop() {

	defer func() {
		// Cancel ticker for watchdog message
		if dp.watchdogTicker != nil {
			dp.watchdogTicker.Stop()
		}

		// Connection is closed in the event loop
	}()

	// Initialize the watchdog ticker
	dp.watchdogTicker = time.NewTicker(60 * time.Second)

	for {
		select {

		case <-dp.watchdogTicker.C:
			if dp.status == StatusEngaged {
				dp.eventLoopChannel <- WatchdogMsg{}
			} else {
				// Got a tick but still not engaged -> Something went wrong. Set Down
				dp.eventLoopChannel <- PeerSetDownCommandMsg{err: fmt.Errorf("CER/CEA not finished before first watchdog event")}
			}

		case in := <-dp.eventLoopChannel:

			switch v := in.(type) {

			// Connect goroutine reports connection established
			// Start the event loop and CER/CEA handshake
			case ConnectionEstablishedMsg:

				config.GetLogger().Debugf("connection established with %s", v.connection.RemoteAddr().String())

				dp.connection = v.connection
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

				// Send the message to the peer. If no answer before first watchdog tick, will SetDown
				dp.eventLoopChannel <- EgressDiameterMsg{message: cer}

			// Connect goroutine reports connection could not be established
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ConnectionErrorMsg:

				config.GetLogger().Errorf("connection error %s", v.err)
				dp.status = StatusTerminated
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: v.err}
				return

			// readLoop goroutine reports the connection is closed
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ReadEOFMsg:

				if dp.status <= StatusTerminated {
					config.GetLogger().Debugf("connection terminated by remote peer %s", dp.connection.RemoteAddr().String())
				} else {
					config.GetLogger().Errorf("connection terminated with remote peer %s", dp.connection.RemoteAddr().String())
				}

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated

				// Cancels all outstanding requests with error
				dp.cancelAll()

				// Tell the router that we are down
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: nil}
				return

			// readLoop goroutine reports a read error
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the Router must recycle it
			case ReadErrorMsg:

				if dp.status <= StatusTerminated {
					config.GetLogger().Errorf("connection read error %v with remote peer %s", v.err, dp.connection.RemoteAddr().String())
				} else {
					config.GetLogger().Debugf("connection terminating with remote peer %s. Last error %v", dp.connection.RemoteAddr().String(), v.err)
				}

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated

				// Cancels all outstanding requests with error
				dp.cancelAll()

				// Tell the router we are down
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: v.err}

				return

			// Same for writes
			case WriteErrorMsg:

				config.GetLogger().Errorf("write error %s with remote peer %s", v.err, dp.connection.RemoteAddr().String)

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.status = StatusTerminated

				// Cancels all outstanding requests with error
				dp.cancelAll()

				// Tell the router we are down
				dp.eventLoopChannel <- PeerDownEvent{Sender: dp, Error: v.err}

				return

			case PeerUpMsg:
				dp.status = StatusEngaged

				// Tell the Router we are up
				dp.routerControlChannel <- PeerUpEvent{Sender: dp, DiameterHost: v.diameterHost}

				// Reinitialize watchdog timer with final value
				dp.watchdogTicker.Stop()
				dp.watchdogTicker = time.NewTicker(time.Duration(dp.peerConfig.WatchdogIntervalMillis) * time.Millisecond)

			// Initiate closing procedure
			case PeerSetDownCommandMsg:

				config.GetLogger().Debug("processing PeerSetDownCommandMsg")

				dp.status = StatusTerminated

				// In case it was still connecting
				if dp.cancel != nil {
					dp.cancel()
				}

				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.cancelAll()

				// Tell the Router we are finished
				dp.routerControlChannel <- PeerDownEvent{Sender: dp, Error: v.err}

				return

				// Send a message to the peer. May be a request or an answer
			case EgressDiameterMsg:

				if dp.status == StatusConnected || dp.status == StatusEngaged {

					// Check not duplicate
					hbhId := v.message.HopByHopId
					if _, ok := dp.requestsMap[hbhId]; ok && v.rchan != nil {
						v.rchan <- fmt.Errorf("duplicated HopByHopId")
						close(v.rchan)
						break
					}

					config.GetLogger().Debugf("-> Sending Message %s\n", v.message)
					_, err := v.message.WriteTo(dp.connection)
					if err != nil {
						// There was an error writing. Will close the connection
						if dp.status <= StatusTerminated {
							dp.eventLoopChannel <- WriteErrorMsg{err}
							dp.status = StatusTerminated
						}

						// Signal the error in the response channel for the input request
						// Do all necessary things to cancell the request
						if v.message.IsRequest && v.rchan != nil {
							v.rchan <- err
							close(v.rchan)
						}

						// No statistics, because the Peer will die

						break
					}

					// All good.
					// If it was a Request, store in the outstanding request map
					// RChan may be nil if it is a base application message
					if v.message.IsRequest {
						instrumentation.PushPeerDiameterRequestSent(dp.peerConfig.DiameterHost, v.message)
						if v.rchan != nil {
							// Set timer
							dp.wg.Add(1)
							timer := time.AfterFunc(v.timeout, func() {
								// This will be called if the timer expires
								dp.eventLoopChannel <- CancelRequestMsg{hopByHopId: v.message.HopByHopId, reason: fmt.Errorf("Timeout")}
								defer dp.wg.Done()
							})

							// Add to requests map
							dp.requestsMap[v.message.HopByHopId] = RequestContext{rchan: v.rchan, timer: timer, key: instrumentation.PeerDiameterMetricFromMessage(dp.peerConfig.DiameterHost, v.message)}
						}
					} else {
						instrumentation.PushPeerDiameterAnswerSent(dp.peerConfig.DiameterHost, v.message)
					}

				} else {
					config.GetLogger().Errorf("%s %s message was not sent because status is %d", v.message.ApplicationName, v.message.CommandName, dp.status)
					if v.rchan != nil {
						v.rchan <- fmt.Errorf("message not sent. Status is not Engaged")
						close(v.rchan)
					}
				}

				// Received message from peer
			case IngressDiameterMsg:

				config.GetLogger().Debugf("<- Receiving Message %s\n", v.message)

				if v.message.IsRequest {

					instrumentation.PushPeerDiameterRequestReceived(dp.peerConfig.DiameterHost, v.message)

					// Check if it is a Base application message (code for Base application is 0)
					if v.message.ApplicationId == 0 {
						switch v.message.CommandName {

						case "Capabilities-Exchange":
							if originHost, err := dp.handleCER(v.message); err != nil {
								// There was an error
								dp.eventLoopChannel <- PeerSetDownCommandMsg{err: err}
								dp.status = StatusTerminated
							} else {
								// The router must check that there is no other connection for the same peer
								// and set state to active
								dp.eventLoopChannel <- PeerUpMsg{diameterHost: originHost}
							}

						case "Device-Watchdog":
							dwa := diamcodec.NewDiameterAnswer(v.message)
							dwa.AddOriginAVPs(dp.ci)
							dwa.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
							dp.eventLoopChannel <- EgressDiameterMsg{message: dwa}

						case "Disconnect-Peer":
							dpa := diamcodec.NewDiameterAnswer(v.message)
							dpa.AddOriginAVPs(dp.ci)
							dp.eventLoopChannel <- EgressDiameterMsg{message: dpa}
							dp.eventLoopChannel <- PeerSetDownCommandMsg{err: fmt.Errorf("received disconnect-peer")}
							dp.status = StatusTerminated

						default:
							config.GetLogger().Warnf("command %d for base applicaton not found in dictionary", v.message.CommandCode)
						}

					} else {
						// Reveived a non base request. Invoke handler
						// Make sure the eventLoopChannel is not closed until the response is received
						dp.wg.Add(1)
						go func() {
							defer dp.wg.Done()
							resp, err := dp.handler(v.message)
							if err != nil {
								config.GetLogger().Error("error handling diameter message: " + err.Error())
								// Send an error UNABLE_TO_COMPLY
								errorResp := diamcodec.NewDiameterAnswer(v.message)
								errorResp.AddOriginAVPs(dp.ci)
								errorResp.Add("Result-Code", diamcodec.DIAMETER_UNABLE_TO_COMPLY)
								dp.eventLoopChannel <- EgressDiameterMsg{message: errorResp}
							} else {
								dp.eventLoopChannel <- EgressDiameterMsg{message: resp}
							}
						}()
					}
				} else {
					// Received an answer
					instrumentation.PushPeerDiameterAnswerReceived(dp.peerConfig.DiameterHost, v.message)

					if v.message.ApplicationId == 0 {
						// Base answer
						switch v.message.CommandName {
						case "Capabilities-Exchange":
							doDisconnect := true
							// Received capabilities exchange answer
							originHostAVP, err := v.message.GetAVP("Origin-Host")
							if err != nil {
								config.GetLogger().Errorf("error getting Origin-Host %s", err)
							} else if originHostAVP.GetString() != dp.peerConfig.DiameterHost {
								config.GetLogger().Errorf("error in CER. Got origin host %s instead of %s", originHostAVP.GetString(), dp.peerConfig.DiameterHost)
							} else if v.message.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
								config.GetLogger().Errorf("error in CER. Got Result code %d", v.message.GetResultCode())
							} else {
								// All good.
								doDisconnect = false
							}

							if doDisconnect {
								dp.status = StatusTerminated
								dp.eventLoopChannel <- PeerSetDownCommandMsg{err: fmt.Errorf("CER/CEA error %w", err)}
							} else {
								dp.eventLoopChannel <- PeerUpMsg{diameterHost: dp.peerConfig.DiameterHost}
							}

						case "Device-Watchdog":
							config.GetLogger().Debug("received dwa")
							if v.message.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
								config.GetLogger().Errorf("bad result code in answer to DWR: %d", v.message.GetResultCode())
								dp.status = StatusTerminated
								dp.eventLoopChannel <- PeerSetDownCommandMsg{err: fmt.Errorf("watchdog answer is not DIAMETER_SUCCESS")}
							} else {
								dp.outstandingDWA--
							}
						default:
							config.GetLogger().Warnf("command %d for base applicaton not found in dictionary", v.message.CommandCode)
						}
					} else {
						// Non base answer
						if requestContext, ok := dp.requestsMap[v.message.HopByHopId]; !ok {
							// Request not found in the requests map
							instrumentation.PushPeerDiameterAnswerStalled(dp.peerConfig.DiameterHost, v.message)
							config.GetLogger().Errorf("stalled diameter answer: '%v'", *v.message)
						} else {
							// Cancel timer
							if requestContext.timer.Stop() {
								// The after func has not been called
								dp.wg.Done()
							} else {
								// Drain the channel so that the tick is not read by anybody else
								// https://itnext.io/go-timer-101252c45166
								select {
								case <-requestContext.timer.C:
								default:
								}
							}
							// Send the response
							requestContext.rchan <- v.message
							close(requestContext.rchan)
							delete(dp.requestsMap, v.message.HopByHopId)
						}
					}
				}

			case CancelRequestMsg:
				config.GetLogger().Debugf("Cancelling HopByHopId: <%d>\n", v.hopByHopId)
				requestContext, ok := dp.requestsMap[v.hopByHopId]
				if !ok {
					config.GetLogger().Errorf("attempt to cancel an non existing request with HopByHopId %d", v.hopByHopId)
				} else {
					// Send the response
					requestContext.rchan <- v.reason
					// No more messages will be sent through this channel
					close(requestContext.rchan)
					// Delete the requestmap entry
					delete(dp.requestsMap, v.hopByHopId)
					// Update metric
					instrumentation.PushPeerDiameterRequestTimeout(requestContext.key)
				}

			case WatchdogMsg:
				config.GetLogger().Debugf("dwr tick")

				// Here we do the checking of the DWA that are pending
				if dp.outstandingDWA > MAX_UNANSWERED_WATCHDOG_REQUESTS {
					config.GetLogger().Errorf("too many unanswered DWR: %d", MAX_UNANSWERED_WATCHDOG_REQUESTS)
					dp.status = StatusTerminated
					dp.eventLoopChannel <- PeerSetDownCommandMsg{err: fmt.Errorf("too many unasnwered DWR")}
				}

				// Create request
				dwr, err := diamcodec.NewDiameterRequest("Base", "Device-Watchdog")
				dwr.AddOriginAVPs(dp.ci)
				if err != nil {
					panic("could not create a DWR")
				}
				dp.eventLoopChannel <- EgressDiameterMsg{message: dwr}
				dp.outstandingDWA++
			}
		}
	}

}

// Establishes the connection with the peer
// To be executed in a goroutine
// Should not touch inner variables
func (dp *DiameterPeer) connect(timeout time.Duration, ipAddress string, port int) {

	// Create a cancellable deadline
	context, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	dp.cancel = cancel

	defer dp.wg.Done()

	// Connect
	var dialer net.Dialer
	if conn, err := dialer.DialContext(context, "tcp4", fmt.Sprintf("%s:%d", ipAddress, port)); err != nil {
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
		if _, err := dm.ReadFrom(dp.connection); err != nil {
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
			dp.eventLoopChannel <- IngressDiameterMsg{message: &dm}
		}
	}

	// Signal that we are finished
	close(ch)
}

// Sends a Diameter request and gets the answer or error as a message to the specified channel.
// The response channel is closed just after sending the reponse or error
func (dp *DiameterPeer) DiameterExchange(dm *diamcodec.DiameterMessage, timeout time.Duration, rchan chan interface{}) {

	if cap(rchan) < 1 {
		panic("using an unbuffered response channel")
	}

	// Validations
	if dm.ApplicationId == 0 {
		rchan <- fmt.Errorf("should not use this method to send a Base Application message")
		return
	}
	if dp.status != StatusEngaged {
		rchan <- fmt.Errorf("tried to send a diameter request in a non engaged DiameterPeer. Status is %d", dp.status)
		return
	}
	if !(*dm).IsRequest {
		rchan <- fmt.Errorf("diameter message is not a request")
		return
	}

	// Send myself the message
	dp.eventLoopChannel <- EgressDiameterMsg{message: dm, rchan: rchan, timeout: timeout}
}

// Handle received CER message, sending the CEA that may be successful or not
// This is executed in the eventLoop
// Returns the Origin-Host received
func (dp *DiameterPeer) handleCER(request *diamcodec.DiameterMessage) (string, error) {

	if dp.status != StatusConnected {
		return "", fmt.Errorf("received CER when status in not connected, but %d", dp.status)
	}

	// Check at least that the peer exists and the origin IP address is valMid
	originHostAVP, err := request.GetAVP("Origin-Host")
	if err == nil {
		originHost := originHostAVP.GetString()
		peersConf := dp.ci.PeersConf()

		remoteAddr, _, _ := net.SplitHostPort(dp.connection.RemoteAddr().String())
		remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)

		if peersConf.ValidateIncomingAddress(originHost, remoteIPAddr.IP) {

			if peerConfig, found := peersConf[originHost]; found {
				// Grab the peer configuration
				dp.peerConfig = peerConfig

				cea := diamcodec.NewDiameterAnswer(request)
				cea.AddOriginAVPs(dp.ci)
				cea.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
				dp.pushCEAttributes(cea)
				dp.eventLoopChannel <- EgressDiameterMsg{message: cea}

				// All good returns here
				return originHost, nil
			} else {
				config.GetLogger().Errorf("Origin-Host not found in configuration %s while handling CER", originHost)
			}
		} else {
			config.GetLogger().Errorf("invalid diameter peer %s with address %s while handling CER", originHost, remoteIPAddr.IP)
		}
	} else {
		config.GetLogger().Errorf("error getting Origin-Host %s while handling CER", err)
	}

	// Send error message before disconnecting
	cea := diamcodec.NewDiameterAnswer(request)
	cea.AddOriginAVPs(dp.ci)
	cea.Add("Result-Code", diamcodec.DIAMETER_UNKNOWN_PEER)
	dp.eventLoopChannel <- EgressDiameterMsg{message: cea}

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
	cer.Add("Origin-State-Id", diamcodec.GetStateId(false, true))
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

// Cancels all Diameter requests. To be executed in the event loop
func (dp *DiameterPeer) cancelAll() {
	// Cancellation of all outstanding requests
	for hopId := range dp.requestsMap {
		config.GetLogger().Debugf("cancelling request %d", hopId)
		requestContext := dp.requestsMap[hopId]

		// Cancel timer
		if requestContext.timer.Stop() {
			// The after func has not been called
			dp.wg.Done()
		} else {
			// Drain the channel so that the tick is not read by anybody else
			// https://itnext.io/go-timer-101252c45166
			select {
			case <-requestContext.timer.C:
			default:
			}
		}
		// Send the error
		requestContext.rchan <- fmt.Errorf("request cancelled due to Peer down")
		close(requestContext.rchan)
		delete(dp.requestsMap, hopId)
	}
}

// For testing purpuses only
func (dp *DiameterPeer) tstForceSocketError() {
	dp.connection.Close()
}

// Forces sending a disconnect message to the connected peer
func (dp *DiameterPeer) tstSendDisconnectPeer() {
	dpm, _ := diamcodec.NewDiameterRequest("Base", "Disconnect-Peer")
	dp.eventLoopChannel <- EgressDiameterMsg{message: dpm}
}
