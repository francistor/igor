package diampeer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"igor/config"
	"io"
	"net"
	"time"
)

const (
	StatusCreated    = 0
	StatusConnecting = 1
	StatusConnected  = 2
	StatusClosing    = 3
	StatusClosed     = 4
)

// Ouput Events
type SocketDownEvent struct {
	Sender *PeerSocket
}
type SocketErrorEvent struct {
	Sender *PeerSocket
	Error  error
}
type SocketConnectedEvent struct {
	Sender *PeerSocket
}

// Intput Commands
type SocketCloseCommand struct{}

// Internal messages
type ConnectionEstablishedMsg net.Conn
type ConnectionErrorMsg error
type ReadEOFMsg struct{}
type ReadErrorMsg struct {
	Error error
}

// A PeerSocket is a helper for doing the low level job of sending messages and
// receiving messages
type PeerSocket struct {

	// Input and output channels
	// Created iternally
	InputChannel chan interface{}

	// Passed as parameter
	OutputChannel chan interface{}

	// To signal graceful closing
	Status int

	// Internal
	connection net.Conn
	connReader *bufio.Reader
	connWriter *bufio.Writer

	// Canceller of connection
	cancel context.CancelFunc
}

func NewActivePeerSocket(oc chan interface{}, connTimeoutMillis int, ipAddress string, port int) PeerSocket {

	// Create the socket
	peerSocket := PeerSocket{InputChannel: make(chan interface{}), OutputChannel: oc}

	peerSocket.Status = StatusConnecting

	go peerSocket.connect(connTimeoutMillis, ipAddress, port)
	go peerSocket.eventLoop()

	return peerSocket
}

func NewPassivePeerSocket(oc chan interface{}, conn net.Conn) PeerSocket {
	// Create the socket
	peerSocket := PeerSocket{InputChannel: make(chan interface{}), OutputChannel: oc, connection: conn}

	peerSocket.Status = StatusConnected

	peerSocket.connReader = bufio.NewReader(peerSocket.connection)
	peerSocket.connWriter = bufio.NewWriter(peerSocket.connection)
	go peerSocket.readLoop()

	go peerSocket.eventLoop()

	return peerSocket
}

// Event loop
func (ps *PeerSocket) eventLoop() {

	defer func() {
		// Close the channels
		close(ps.InputChannel)

		// Close the connection (another time, should not make harm)
		if ps.connection != nil {
			ps.connection.Close()
		}
	}()

	for {
		in := <-ps.InputChannel

		switch v := in.(type) {
		// connect goroutine reports connection established
		case ConnectionEstablishedMsg:
			ps.connection = v
			ps.connReader = bufio.NewReader(ps.connection)
			ps.connWriter = bufio.NewWriter(ps.connection)
			go ps.readLoop()
			ps.OutputChannel <- SocketConnectedEvent{Sender: ps}
			ps.Status = StatusConnected

			// connect goroutine reports connection could not be established
		case ConnectionErrorMsg:
			if ps.Status <= StatusClosing {
				ps.OutputChannel <- SocketErrorEvent{Sender: ps, Error: v}
			}
			ps.OutputChannel <- SocketDownEvent{Sender: ps}
			ps.Status = StatusClosed
			return

			// readLoop goroutine reports the connection is closed
		case ReadEOFMsg:
			ps.OutputChannel <- SocketDownEvent{Sender: ps}
			ps.Status = StatusClosed
			return

			// readLoop goroutine reports a read error
		case ReadErrorMsg:
			if ps.Status < StatusClosing {
				ps.OutputChannel <- SocketErrorEvent{Sender: ps}
			}
			ps.OutputChannel <- SocketDownEvent{Sender: ps}
			ps.Status = StatusClosed
			return

			// command received from outside
		case SocketCloseCommand:

			// In case it was still connecting
			ps.cancel()

			// Close the connection. Any reads will return
			if ps.connection != nil {
				ps.connection.Close()
			}

			ps.Status = StatusClosing

			// The readLoop goroutine will report the connection has been closed

			// Send a message to the peer
		case []byte:
			if ps.Status == StatusConnected {
				ps.connection.Write(v)
			} else {
				config.IgorLogger.Error("message was not sent because status is %d", ps.Status)
			}
		}
	}

}

// Establishes the connection with the peer
// To be executed in a goroutine
// Should not touch inner variables
func (ps *PeerSocket) connect(connTimeoutMillis int, ipAddress string, port int) {

	// Create a cancellable deadline
	context, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(connTimeoutMillis)*time.Millisecond))
	ps.cancel = cancel
	defer ps.cancel()

	// Connect
	var dialer net.Dialer
	conn, err := dialer.DialContext(context, "tcp4", fmt.Sprintf("%s:%d", ipAddress, port))

	if err != nil {
		ps.InputChannel <- ConnectionErrorMsg(err)
	} else {
		ps.InputChannel <- ConnectionEstablishedMsg(conn)
	}

}

// Reader of peer messages
// To be executed in a goroutine
// Should not touch inner variables
func (ps *PeerSocket) ReadLoop2() {
	for {
		// Each iteration will create a new buffer. Not shared
		var receivedBytes = make([]byte, 1024)
		_, err := ps.connection.Read(receivedBytes)
		if err != nil {
			if err == io.EOF {
				// The remote peer closed
				ps.InputChannel <- ReadEOFMsg{}
			} else {
				ps.InputChannel <- ReadErrorMsg{err}
			}
			return
		}

		ps.OutputChannel <- receivedBytes
	}
}

// Reader of peer messages
// To be executed in a goroutine
// Should not touch inner variables
func (ps *PeerSocket) readLoop() {
	for {
		// Read version and size
		// First four bytes
		var initialBuffer = make([]byte, 4)
		_, err := io.ReadAtLeast(ps.connReader, initialBuffer, 4)
		initialBuffer[0] = 0 // First byte is the version
		size := uint32(binary.BigEndian.Uint32(initialBuffer))
		if err != nil {
			if err == io.EOF {
				// The remote peer closed
				ps.InputChannel <- ReadEOFMsg{}
			} else {
				ps.InputChannel <- ReadErrorMsg{err}
			}
			return
		}

		// Read all the message
		// var size = firstWord & 16777215 // 2^24 - 1
		var buffer = make([]byte, size)
		_, err = io.ReadAtLeast(io.MultiReader(bytes.NewReader(initialBuffer), ps.connReader), buffer, int(size))
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// The remote peer closed
				ps.InputChannel <- ReadEOFMsg{}
			} else {
				ps.InputChannel <- ReadErrorMsg{err}
			}
			return
		}

		ps.OutputChannel <- buffer
	}
}
