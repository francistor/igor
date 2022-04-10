package diampeer

import (
	"bufio"
	"context"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"io"
	"net"
	"time"
)

const (
	StatusConnecting = 1
	StatusConnected  = 2
	StatusEngaged    = 3
	StatusClosing    = 4
	StatusClosed     = 5
)

// Ouput Events
type PeerDownEvent struct {
	// Myself
	Sender *DiameterPeer
	// Will be nil if the reason is not an error
	Error error
}

type PeerUpEvent struct {
	// Myself
	Sender *DiameterPeer
	// Reported identity of the remote peer
	DiameterHost string
}

// Message from me to a Diameteer Peer. May be a Request or an Answer
type EgressDiameterMessage struct {
	Message *diamcodec.DiameterMessage
	// Channel to receive the Answer or nil
	RChann *chan interface{}
}

// Message received from a Diameter Peer. May be a Request or an Answer
type IngressDiameterMessage struct {
	Message *diamcodec.DiameterMessage
}

// Timeout expired
type CancelDiameterRequest struct {
	HopByHopId uint32
}

// Internal messages
type PeerCloseCommand struct{}

type ConnectionEstablishedMsg struct {
	Connection net.Conn
}
type ConnectionErrorMsg struct {
	Error error
}
type ReadEOFMsg struct{}
type ReadErrorMsg struct {
	Error error
}
type WriteErrorMsg struct {
	Error error
}

type MessageHandler func(request *diamcodec.DiameterMessage) (answer *diamcodec.DiameterMessage, err error)

// This object abstracts the operations against a Diameter Peer
// It implements the Actor model: all internal variables are modified
// from an internal single threaded EventLoop and message passing
type DiameterPeer struct {

	// Holds the Peer configuration
	// Passed during instantiation if Peer is Active
	// TODO: Filled after CER/CEA exchange and settlement of duplicates
	PeerConfig config.DiameterPeer

	// Input and output channels

	// Created iternally
	eventLoopChannel chan interface{}

	// Passed as parameter. To report events to the Router
	OutputChannel chan interface{}

	// The Status of the object (one of the const defined)
	Status int

	// Internal
	connection net.Conn
	connReader *bufio.Reader
	connWriter *bufio.Writer

	// Canceller of connection
	cancel context.CancelFunc

	// Outstanding requests map
	// Maps HopByHopIds to a channel where the response or a timeout will be sent
	requestsMap map[uint32]*chan interface{}

	// Handler
	handler MessageHandler
}

// Creates a new DiameterPeer when we are expected to establish the connection with the other side
func NewActiveDiameterPeer(oc chan interface{}, peer config.DiameterPeer, handler MessageHandler) *DiameterPeer {

	config.IgorLogger.Debugf("creating active diameter peer for %s", peer.DiameterHost)

	// Create the Peer struct
	diamPeer := DiameterPeer{eventLoopChannel: make(chan interface{}), OutputChannel: oc, PeerConfig: peer, requestsMap: make(map[uint32]*chan interface{}), handler: handler}

	diamPeer.Status = StatusConnecting

	// Default value for timeout
	timeout := peer.ConnectionTimeoutMillis
	if timeout == 0 {
		timeout = 5000
	}
	go diamPeer.connect(timeout, peer.IPAddress, peer.Port)

	go diamPeer.eventLoop()

	return &diamPeer
}

// Creates a new DiameterPeer when the connection has been accepted already
func NewPassiveDiameterPeer(oc chan interface{}, conn net.Conn, handler MessageHandler) *DiameterPeer {

	config.IgorLogger.Debugf("creating passive diameter peer for %s", conn.RemoteAddr().String())

	// Create the socket
	diamPeer := DiameterPeer{eventLoopChannel: make(chan interface{}), OutputChannel: oc, connection: conn, requestsMap: make(map[uint32]*chan interface{}), handler: handler}

	diamPeer.Status = StatusConnected

	diamPeer.connReader = bufio.NewReader(diamPeer.connection)
	diamPeer.connWriter = bufio.NewWriter(diamPeer.connection)
	go diamPeer.readLoop()

	go diamPeer.eventLoop()

	return &diamPeer
}

// Terminates the Peer connection and the event loop
// The object may be recycled
func (dp *DiameterPeer) Close() {
	dp.eventLoopChannel <- PeerCloseCommand{}
}

// Event loop
func (dp *DiameterPeer) eventLoop() {

	defer func() {
		// Close the channels that we have created
		close(dp.eventLoopChannel)

		// Close the connection (another time, should not make harm)
		if dp.connection != nil {
			dp.connection.Close()
		}
	}()

	for {
		in := <-dp.eventLoopChannel

		switch v := in.(type) {
		// Connect goroutine reports connection established
		// Start the event loop
		case ConnectionEstablishedMsg:
			config.IgorLogger.Debug("connection established")
			dp.connection = v.Connection
			dp.connReader = bufio.NewReader(dp.connection)
			dp.connWriter = bufio.NewWriter(dp.connection)
			go dp.readLoop()

			// TODO: Send this after CER/CEA handshake
			dp.OutputChannel <- PeerUpEvent{Sender: dp}
			dp.Status = StatusConnected

		// Connect goroutine reports connection could not be established
		// the DiameterPeer will terminate the event loop, send the Down event
		// and the router must recycle it
		case ConnectionErrorMsg:
			config.IgorLogger.Debugf("connection error %s", v.Error)
			dp.OutputChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
			dp.Status = StatusClosed
			return

		// readLoop goroutine reports the connection is closed
		// the DiameterPeer will terminate the event loop, send the Down event
		// and the router must recycle it
		case ReadEOFMsg:
			config.IgorLogger.Debug("Read EOF")
			dp.OutputChannel <- PeerDownEvent{Sender: dp, Error: nil}
			dp.Status = StatusClosed
			return

		// readLoop goroutine reports a read error
		// the DiameterPeer will terminate the event loop, send the Down event
		// and the router must recycle it
		case ReadErrorMsg:
			config.IgorLogger.Debugf("read error %s", v.Error)
			dp.OutputChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
			dp.Status = StatusClosed
			return

		// Same for writes
		case WriteErrorMsg:
			config.IgorLogger.Debugf("write error %s", v.Error)
			dp.OutputChannel <- PeerDownEvent{Sender: dp, Error: v.Error}
			dp.Status = StatusClosed
			return

		// command received from outside
		case PeerCloseCommand:
			config.IgorLogger.Debugf("closing")
			// In case it was still connecting
			if dp.cancel != nil {
				dp.cancel()
			}

			// Close the connection. Any reads will return
			if dp.connection != nil {
				dp.connection.Close()
			}

			dp.Status = StatusClosing

			// The readLoop goroutine will report the connection has been closed

			// Send a message to the peer. May be a request or an answer
		case EgressDiameterMessage:
			if dp.Status == StatusConnected {
				config.IgorLogger.Debugf("-> Sending Message %s\n", v.Message)
				_, err := v.Message.WriteTo(dp.connection)
				if err != nil {
					dp.eventLoopChannel <- WriteErrorMsg{err}
					if v.Message.IsRequest {
						*v.RChann <- err
					}
				}

				// If it was a Request, store in the outstanding request map
				if v.Message.IsRequest {
					dp.requestsMap[v.Message.HopByHopId] = v.RChann
				}

			} else {
				config.IgorLogger.Error("message was not sent because status is %d", dp.Status)
			}

			// Received message from peer
		case IngressDiameterMessage:
			config.IgorLogger.Debugf("<- Receiving Message %s\n", v.Message)
			if v.Message.IsRequest {
				// Reveived a request. Invoke handler
				go func() {
					resp, err := dp.handler(v.Message)
					if err != nil {
						config.IgorLogger.Error(err)
					} else {
						dp.eventLoopChannel <- EgressDiameterMessage{Message: resp}
					}
				}()
			} else {
				// Received an answer
				respChann, ok := dp.requestsMap[v.Message.HopByHopId]
				if !ok {
					config.IgorLogger.Errorf("stalled diameter answer: '%v'", *v.Message)
				} else {
					*respChann <- v.Message
					close(*respChann)
					delete(dp.requestsMap, v.Message.HopByHopId)
				}
			}

		case CancelDiameterRequest:
			config.IgorLogger.Debugf("Timeout to %d\n", v.HopByHopId)
			respChann, ok := dp.requestsMap[v.HopByHopId]
			if !ok {
				config.IgorLogger.Errorf("attemtp to cancel an non existing request")
			} else {
				close(*respChann)
				delete(dp.requestsMap, v.HopByHopId)
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
func (dp *DiameterPeer) readLoop() {
	for {
		// Read a Diameter message from the connection
		dm := diamcodec.DiameterMessage{}
		_, err := dm.ReadFrom(dp.connReader)

		if err != nil {
			if err == io.EOF {
				// The remote peer closed
				dp.eventLoopChannel <- ReadEOFMsg{}
			} else {
				dp.eventLoopChannel <- ReadErrorMsg{err}
			}
			return
		}

		// Send myself the received message
		dp.eventLoopChannel <- IngressDiameterMessage{Message: &dm}
	}
}

// Sends a Diameter request and gets the answer or an error (timeout or network error)
func (dp *DiameterPeer) DiameterRequest(dm *diamcodec.DiameterMessage, timeout time.Duration) (resp *diamcodec.DiameterMessage, e error) {

	// Validations
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

//Code for Sending CER and, in general, Base messages
