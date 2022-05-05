package diampeer

import (
	"bufio"
	"context"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"io"
	"net"
	"strings"
	"time"
)

const (
	StatusConnecting = 1
	StatusConnected  = 2
	StatusEngaged    = 3
	StatusClosing    = 4 // No more requests allowed
	StatusClosed     = 5 // EventLoop not running
)

const (
	EVENTLOOP_CAPACITY = 100
)

// Ouput Events (control channel)

// Sent to the router, via the output channel passed as parameter, to signal
// that the Peer object is down and should be recycled
// If the reason is an error (e.g. bad response from the other, communication problem),
// etc. the Error field will be not null
type PeerDownEvent struct {
	// Myself
	Sender *DiameterPeer
	// Will be nil if the reason is not an error
	Error error
}

// Sent to the router, via the output channel passed as parameter, to signal
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
// If a request, RChann will contain the channel on which the answer
// must be written
type EgressDiameterMessage struct {
	Message *diamcodec.DiameterMessage
	// nil if a Response
	RChann *chan interface{}
}

// Message received from a Diameter Peer. May be a Request or an Answer
type IngressDiameterMessage struct {
	Message *diamcodec.DiameterMessage
}

// Timeout expired waiting for a Diameter Answer
// The HopByHopId will hold the key in the requestsMap
type CancelDiameterRequest struct {
	HopByHopId uint32
}

// Send internally to force a disconnection, moving the Peer to
// the closed state
type PeerCloseCommand struct{}

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

type MessageHandler func(request *diamcodec.DiameterMessage) (answer *diamcodec.DiameterMessage, err error)

// This object abstracts the operations against a Diameter Peer
// It implements the Actor model: all internal variables are modified
// from an internal single threaded EventLoop and message passing

// A DiameterPeer is created using one of the NewXXX methods, passing a control channel. A PeerDown
// will always be sent, either because the peer engaging process did not terminate correctly, because
// an error reading or writting from the TCP socket happens, or due to explicit termination (Disengage method).
// The DiameterPeer object is then set to "Closed" state, but the Close() method must be called explicitly
// to close the internal channel for the event loop
// After the engagement process terminates correctly, the PeerUp event is sent through the control channel

type DiameterPeer struct {

	// Holds the configuration instance for this DiamterPeer
	ci *config.ConfigurationManager

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
	// can be closed
	readLoopChannel chan bool

	// Passed as parameter. To report events to the Router
	ControlChannel chan interface{}

	// The Status of the object (one of the const defined)
	status int

	// Internal
	connection net.Conn
	connReader *bufio.Reader
	connWriter *bufio.Writer

	// Canceller of connection
	cancel context.CancelFunc

	// Outstanding requests map
	// Maps HopByHopIds to a channel where the response or a timeout will be sent
	requestsMap map[uint32]*chan interface{}

	// Registered Handler for incoming messages
	handler MessageHandler

	// Ticker for watchdog requests
	watchdogTicker *time.Ticker
}

// Creates a new DiameterPeer when we are expected to establish the connection with the other side
// and initiate the CER/CEA handshake
// TODO: Treat gracefully the case where the peer is active but receives a CER message
func NewActiveDiameterPeer(configInstanceName string, oc chan interface{}, peer config.DiameterPeer, handler MessageHandler) *DiameterPeer {

	// Create the Peer struct
	dp := DiameterPeer{ci: config.GetConfigInstance(configInstanceName), eventLoopChannel: make(chan interface{}, EVENTLOOP_CAPACITY), ControlChannel: oc, PeerConfig: peer, requestsMap: make(map[uint32]*chan interface{}), handler: handler}

	dp.ci.IgorLogger.Debugf("creating active diameter peer for %s", peer.DiameterHost)

	dp.status = StatusConnecting

	// Default value for timeout
	timeout := peer.ConnectionTimeoutMillis
	if timeout == 0 {
		timeout = 5000
	}

	// This will send a ConnectionEstablishedMsg or ConnectionErrorMsg
	go dp.connect(timeout, peer.IPAddress, peer.Port)

	go dp.eventLoop()

	return &dp
}

// Creates a new DiameterPeer when the connection has been alread accepted
func NewPassiveDiameterPeer(configInstanceName string, oc chan interface{}, conn net.Conn, handler MessageHandler) *DiameterPeer {

	// Create the Peer Strict
	dp := DiameterPeer{ci: config.GetConfigInstance(configInstanceName), eventLoopChannel: make(chan interface{}, EVENTLOOP_CAPACITY), ControlChannel: oc, connection: conn, requestsMap: make(map[uint32]*chan interface{}), handler: handler}

	dp.ci.IgorLogger.Debugf("creating passive diameter peer for %s", conn.RemoteAddr().String())

	dp.status = StatusConnected

	dp.connReader = bufio.NewReader(dp.connection)
	dp.connWriter = bufio.NewWriter(dp.connection)

	dp.readLoopChannel = make(chan bool)
	go dp.readLoop(dp.readLoopChannel)

	go dp.eventLoop()

	return &dp
}

// Terminates the Peer connection and the event loop
// The object may be recycled
// A PeerDown message will be sent through the control channel
func (dp *DiameterPeer) Disengage() {
	dp.eventLoopChannel <- PeerCloseCommand{}
}

// Closes the event loop channel
// Use this method only after a PeerDown event has been received
func (dp *DiameterPeer) Close() {
	close(dp.eventLoopChannel)
}

// Event Loop
func (dp *DiameterPeer) eventLoop() {

	defer func() {
		// Cancel ticker
		if dp.watchdogTicker != nil {
			dp.watchdogTicker.Stop()
		}

		// Close the connection (another time, should not make harm)
		if dp.connection != nil {
			dp.connection.Close()
		}

		// Wait for the readLoop to stop
		if dp.readLoopChannel != nil {
			<-dp.readLoopChannel
		}
	}()

	// Initialize to something, in order to be able to select below
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

				dp.ci.IgorLogger.Debug("connection established")

				dp.connection = v.Connection
				dp.connReader = bufio.NewReader(dp.connection)
				dp.connWriter = bufio.NewWriter(dp.connection)

				dp.readLoopChannel = make(chan bool)
				go dp.readLoop(dp.readLoopChannel)

				dp.status = StatusConnected

				// Active Peer. We'll send the CER.
				go func() {
					if dp.doCERExchange() != nil {
						dp.status = StatusClosing
						dp.eventLoopChannel <- PeerCloseCommand{}
					} else {
						dp.eventLoopChannel <- PeerUpMsg{DiameterHost: dp.PeerConfig.DiameterHost}
					}
				}()

			// Connect goroutine reports connection could not be established
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the router must recycle it
			case ConnectionErrorMsg:

				dp.ci.IgorLogger.Debugf("connection error %s", v.Error)
				dp.status = StatusClosed
				dp.ControlChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
				return

			// readLoop goroutine reports the connection is closed
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the router must recycle it
			case ReadEOFMsg:

				if dp.status < StatusClosing {
					dp.ci.IgorLogger.Debug("connection terminated by remote peer")
				} else {
					dp.ci.IgorLogger.Error("connection terminated")
				}
				dp.status = StatusClosed
				dp.ControlChannel <- PeerDownEvent{Sender: dp, Error: nil}
				return

			// readLoop goroutine reports a read error
			// the DiameterPeer will terminate the event loop, send the Down event
			// and the router must recycle it
			case ReadErrorMsg:

				if dp.status < StatusClosing {
					dp.ci.IgorLogger.Errorf("read error %s", v.Error)
				} else {
					dp.ci.IgorLogger.Debugf("read error %s", v.Error)
				}
				dp.status = StatusClosed
				dp.ControlChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
				return

			// Same for writes
			case WriteErrorMsg:

				dp.ci.IgorLogger.Errorf("write error %s", v.Error)
				dp.status = StatusClosing
				dp.ControlChannel <- PeerCloseCommand{}

			case PeerUpMsg:
				dp.status = StatusEngaged
				dp.ControlChannel <- PeerUpEvent{Sender: dp, DiameterHost: v.DiameterHost}

				// Reinitialize the timer with the right duration
				dp.watchdogTicker.Stop()
				dp.watchdogTicker = time.NewTicker(time.Duration(dp.PeerConfig.WatchdogIntervalMillis) * time.Millisecond)

			// Initiate closing procedure
			case PeerCloseCommand:

				dp.ci.IgorLogger.Debugf("processing PeerCloseCommand")

				// In case it was still connecting
				if dp.cancel != nil {
					dp.cancel()
				}

				// Close the connection. Any reads will return with error in the read loop, which will terminate
				// and send control message through the readloopChannel
				if dp.connection != nil {
					dp.connection.Close()
				}

				dp.ControlChannel <- PeerDownEvent{Sender: dp}

				dp.status = StatusClosed

				return

				// The readLoop goroutine will report the connection has been closed

				// Send a message to the peer. May be a request or an answer
			case EgressDiameterMessage:

				if dp.status == StatusConnected || dp.status == StatusEngaged {
					dp.ci.IgorLogger.Debugf("-> Sending Message %s\n", v.Message)
					_, err := v.Message.WriteTo(dp.connection)
					if err != nil {
						// There was an error writing. Will close the connection
						dp.eventLoopChannel <- WriteErrorMsg{err}
						dp.status = StatusClosing

						// Signal the error in the response channel for the input request
						if v.Message.IsRequest {
							*v.RChann <- err
						}
					}

					// All good.
					// If it was a Request, store in the outstanding request map
					if v.Message.IsRequest {
						dp.requestsMap[v.Message.HopByHopId] = v.RChann
					}

				} else {
					dp.ci.IgorLogger.Errorf("%s %s message was not sent because status is %d", v.Message.ApplicationName, v.Message.CommandName, dp.status)
				}

				// Received message from peer
			case IngressDiameterMessage:

				dp.ci.IgorLogger.Debugf("<- Receiving Message %s\n", v.Message)

				if v.Message.IsRequest {
					// Check if it is a Base application message (code for Base application is 0)
					if v.Message.ApplicationId == 0 {
						if command, ok := dp.ci.DiameterDict.AppByCode[0].CommandByCode[v.Message.CommandCode]; ok {
							switch command.Name {

							case "Capabilities-Exchange":
								if originHost, err := dp.handleCER(v.Message); err != nil {
									// There was an error
									// dp.status = StatusClosing
									dp.eventLoopChannel <- PeerCloseCommand{}
								} else {
									// The router must check that there is no other connection for the same peer
									// and set state to active
									dp.status = StatusEngaged
									dp.eventLoopChannel <- PeerUpMsg{DiameterHost: originHost}
								}

							case "Device-Watchdog":
								dwa := diamcodec.NewInstanceDiameterAnswer(dp.ci, v.Message)
								dp.eventLoopChannel <- EgressDiameterMessage{Message: &dwa}

							case "Disconnect-Peer":
								dpa := diamcodec.NewInstanceDiameterAnswer(dp.ci, v.Message)
								dp.eventLoopChannel <- EgressDiameterMessage{Message: &dpa}
								dp.eventLoopChannel <- PeerCloseCommand{}
								dp.status = StatusClosing
							}
						}
					} else {
						// Reveived a non base request. Invoke handler
						go func() {
							resp, err := dp.handler(v.Message)
							if err != nil {
								dp.ci.IgorLogger.Error(err)
							} else {
								dp.eventLoopChannel <- EgressDiameterMessage{Message: resp}
							}
						}()
					}
				} else {
					// Received an answer
					if respChann, ok := dp.requestsMap[v.Message.HopByHopId]; !ok {
						dp.ci.IgorLogger.Errorf("stalled diameter answer: '%v'", *v.Message)
					} else {
						*respChann <- v.Message
						close(*respChann)
						delete(dp.requestsMap, v.Message.HopByHopId)
					}
				}

			case CancelDiameterRequest:
				dp.ci.IgorLogger.Debugf("Timeout to HopByHopId: <%d>\n", v.HopByHopId)
				respChann, ok := dp.requestsMap[v.HopByHopId]
				if !ok {
					dp.ci.IgorLogger.Errorf("attempt to cancel an non existing request")
				} else {
					close(*respChann)
					delete(dp.requestsMap, v.HopByHopId)
				}

			case WatchdogMsg:
				dp.ci.IgorLogger.Debugf("sending dwr")
				go func() {
					dwr, err := diamcodec.NewInstanceDiameterRequest(dp.ci, "Base", "Device-Watchdog")
					if err != nil {
						panic("could not create a DWR")
					}
					resp, err := dp.DiameterRequest(&dwr, time.Duration(dp.PeerConfig.ConnectionTimeoutMillis)*time.Millisecond)
					if err != nil {
						dp.ci.IgorLogger.Errorf("error in answer to DWR: %s", err)
						dp.eventLoopChannel <- PeerCloseCommand{}
						dp.status = StatusClosing
					} else if resp.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
						dp.ci.IgorLogger.Errorf("bad result code in answer to DWR: %s", resp.GetResultCode())
						dp.eventLoopChannel <- PeerCloseCommand{}
						dp.status = StatusClosing
					}
				}()
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
	defer dp.cancel()

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
				dp.eventLoopChannel <- ReadErrorMsg{err}
			}
			break
		} else {
			// Send myself the received message
			dp.eventLoopChannel <- IngressDiameterMessage{Message: &dm}
		}
	}

	// End of event loop
	close(ch)
}

// Sends a Diameter request and gets the answer or an error (timeout or network error)
func (dp *DiameterPeer) DiameterRequest(dm *diamcodec.DiameterMessage, timeout time.Duration) (resp *diamcodec.DiameterMessage, e error) {

	// Validations
	if dm.ApplicationId != 0 && dp.status != StatusEngaged {
		return nil, fmt.Errorf("tried to send a diameter request in a non engaged DiameterPeer. Status is %d", dp.status)
	}

	if !(*dm).IsRequest {
		return nil, fmt.Errorf("Diameter message is not a request")
	}
	hbhId := (*dm).HopByHopId
	if _, ok := dp.requestsMap[hbhId]; ok {
		return nil, fmt.Errorf("Duplicated HopByHopId")
	}

	// This channel will receive the response
	// It will be closed in the event loop, at the same time as deleting the requestMap entry
	var responseChannel = make(chan interface{})

	// Send myself the message
	dp.eventLoopChannel <- EgressDiameterMessage{Message: dm, RChann: &responseChannel}

	// Create the timer
	timer := time.NewTimer(timeout)

	// Wait for the timer or the response, which can be a DiameterAnswer or an error
	select {
	case <-timer.C:
		dp.eventLoopChannel <- CancelDiameterRequest{HopByHopId: hbhId}
		return nil, fmt.Errorf("Timeout")

	case r := <-responseChannel:
		switch v := r.(type) {
		case error:
			return nil, v
		case *diamcodec.DiameterMessage:
			return v, nil
		}
	}

	// TODO: Write code in event loop to support this, and finish building this function
	panic("unreachable code in diampeer.DiameterRequest")
}

// Sends the message and executes the handler function when the answer is received
// In case of error, the response will be nill and e will be non nil
func (dp *DiameterPeer) DiameterRequestAsync(dm *diamcodec.DiameterMessage, timeout time.Duration, handler func(resp *diamcodec.DiameterMessage, e error)) {
	go func() {
		handler(dp.DiameterRequest(dm, timeout))
	}()
}

// Base messages handling

// Send CER and get CEA
// To be used with active peer
func (dp *DiameterPeer) doCERExchange() error {
	cer, err := diamcodec.NewInstanceDiameterRequest(dp.ci, "Base", "Capabilities-Exchange")
	if err != nil {
		panic("could not create a CER")
	}
	// Finish building the CER message
	dp.pushCEAttributes(&cer)

	// Reusing the connection timeout as request timeout :-S
	cea, err := dp.DiameterRequest(&cer, time.Duration(dp.PeerConfig.ConnectionTimeoutMillis)*time.Millisecond)
	if err != nil {
		dp.ci.IgorLogger.Errorf("Capabilities-Exchange returned an error: %s", err)
		return fmt.Errorf("bad CER exchange (initiated by me)")
	} else {
		// Check Answer
		// TODO: Compute the intersection of supported applications
		originHostAVP, err := cea.GetAVP("Origin-Host")
		if err != nil || originHostAVP.GetString() != dp.PeerConfig.DiameterHost || cea.GetResultCode() != diamcodec.DIAMETER_SUCCESS {

			// Error in response. Write log and goodbye
			if err != nil {
				dp.ci.IgorLogger.Errorf("error in CER %s", err)
			} else if cea.GetResultCode() != diamcodec.DIAMETER_SUCCESS {
				dp.ci.IgorLogger.Errorf("error in CER. Got Result code %d", cea.GetResultCode())
			} else {
				dp.ci.IgorLogger.Errorf("error in CER. Got origin host %s instead of %s", originHostAVP.GetString(), dp.PeerConfig.DiameterHost)
			}
			return fmt.Errorf("bad CER exchange (initiated by me)")
		}

		// All good
		return nil
	}
}

// Handle received CER message
// May send an error response to the remote peer, but the main action after this
// is executed externaly
func (dp *DiameterPeer) handleCER(request *diamcodec.DiameterMessage) (string, error) {

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
				// Grab the peer configuration. Should be done in the eventloop thread
				dp.PeerConfig = peerConfig

				cea := diamcodec.NewInstanceDiameterAnswer(dp.ci, request)
				cea.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)
				dp.pushCEAttributes(&cea)
				dp.eventLoopChannel <- EgressDiameterMessage{Message: &cea}

				// All good returns here
				return originHost, nil
			} else {
				dp.ci.IgorLogger.Errorf("Origin-Host not found in configuration %s while handling CER", originHost)
				sendErrorMessage = true
			}
		} else {
			dp.ci.IgorLogger.Errorf("invalid diameter peer %s %s while handling CER", remoteIPAddr.IP, originHost)
			sendErrorMessage = true

		}
	} else {
		dp.ci.IgorLogger.Errorf("error getting Origin-Host %s while handling CER", err)
	}

	if sendErrorMessage {
		// Send error message before disconnecting
		cea := diamcodec.NewInstanceDiameterAnswer(dp.ci, request)
		cea.Add("Result-Code", diamcodec.DIAMETER_UNKNOWN_PEER)
		dp.eventLoopChannel <- EgressDiameterMessage{Message: &cea}
	}

	return "", fmt.Errorf("Bad CER/CEA exchange")
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
			if appDict, ok := dp.ci.DiameterDict.AppByName[rule.ApplicationId]; ok {
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
