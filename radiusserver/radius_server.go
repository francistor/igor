package radiusserver

import (
	"fmt"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/francistor/igor/core"
)

// Valid statuses
const (
	StatusOperational = 0
	StatusTerminated  = 1
)

// Implements a radius server socket
// Validates incoming messages, sends them to the router for processing and replies back
// with the responses
// TODO: Being able to update the radius clients
type RadiusServer struct {

	// Radius Clients
	radiusClients core.RadiusClients

	// Handler function for incoming packets
	// handler RadiusPacketHandler
	handler core.RadiusPacketHandler

	// The UDP socket
	socket net.PacketConn

	// Status. Initially 0 and 1 (StatusTerminated) if we are shutting down
	status int32
}

// Creates a Radius Server
func NewRadiusServer(radiusClients core.RadiusClients, bindAddress string, bindPort int, handler core.RadiusPacketHandler) *RadiusServer {

	// Create the server socket
	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindAddress, bindPort))
	if err != nil {
		panic(fmt.Sprintf("could not create listen socket in %s:%d : %s", bindAddress, bindPort, err))
	} else {
		core.GetLogger().Infof("RADIUS server listening in %s:%d", bindAddress, bindPort)
	}

	radiusServer := RadiusServer{
		radiusClients: radiusClients,
		handler:       handler,
		socket:        socket,
	}

	// Start receiving packets
	go radiusServer.readLoop(socket)

	return &radiusServer
}

// Frees the server socket. No need to call any SetDown() here
func (rs *RadiusServer) Close() {
	// Set the status
	atomic.StoreInt32(&rs.status, StatusTerminated)

	// Will generate an error in the loop, and the readerLoop will return
	rs.socket.Close()
}

func (rs *RadiusServer) readLoop(socket net.PacketConn) {

	// Single buffer where all incoming packets are read
	// According to RFC 2865, the maximum packet size is 4096
	reqBuf := make([]byte, 4096)

	for {
		packetSize, clientAddr, err := socket.ReadFrom(reqBuf)
		if err != nil {
			// Check here if the error is due to the socket being closed
			if atomic.LoadInt32(&rs.status) == StatusTerminated {
				// The socket was closed gracefully
				core.GetLogger().Infof("closed radius server socket %s", socket.LocalAddr().String())
				return
			} else {
				// Some other error
				panic(err)
			}
		}

		clientIP := clientAddr.(*net.UDPAddr).IP
		clientIPAddr := clientIP.String()
		radiusClient, err := rs.radiusClients.FindRadiusClient(clientIP)
		if err != nil {
			core.RecordRadiusServerDrop(clientIPAddr, "0")
			core.GetLogger().Warnf("message from unknown client %s", clientIPAddr)
			continue
		}

		// Decode the packet
		core.GetLogger().Debugf("received packet: %v", reqBuf[:packetSize])
		radiusPacket, err := core.NewRadiusPacketFromBytes((reqBuf[:packetSize]), radiusClient.Secret, core.Zero_authenticator)
		if err != nil {
			core.GetLogger().Errorf("error decoding packet %s\n", err)
			continue
		}

		// Validate the request authenticator
		if radiusPacket.Code != core.ACCESS_REQUEST {
			if !core.ValidateRequestAuthenticator(reqBuf[:packetSize], radiusClient.Secret) {
				core.RecordRadiusServerDrop(clientIPAddr, strconv.Itoa(int(radiusPacket.Code)))
				core.GetLogger().Warnf("invalid request packet. Bad authenticator %s\n", radiusPacket)
				continue
			}
		}

		// Validate the message authenticator, if present
		if radiusPacket.HasMessageAuthenticator() {
			if !radiusPacket.ValidateMessageAuthenticator(reqBuf[:packetSize], radiusClient.Secret) {
				core.RecordRadiusServerDrop(clientIPAddr, strconv.Itoa(int(radiusPacket.Code)))
				core.GetLogger().Warnf("invalid request packet. Bad message authenticator %s\n", radiusPacket)
				continue
			}
		}

		core.RecordRadiusServerRequest(clientIPAddr, strconv.Itoa(int(radiusPacket.Code)))
		core.GetLogger().Debugf("<- Server received RadiusPacket %s\n", radiusPacket)

		// Wait for response
		go func(requestPacket *core.RadiusPacket, secret string, addr net.Addr) {

			code := requestPacket.Code

			responsePacket, err := rs.handler(requestPacket)

			if err != nil {
				core.GetLogger().Errorf("discarding packet for %s with code %d: %s", addr.String(), requestPacket.Code, err)
				core.RecordRadiusServerDrop(clientIPAddr, strconv.Itoa(int(code)))
				return
			}

			// Build the response
			respBuf, err := responsePacket.ToBytes(secret, requestPacket.Identifier, core.Zero_authenticator, false)
			if err != nil {
				core.GetLogger().Errorf("error serializing packet for %s with code %d: %s", addr.String(), code, err)
				core.RecordRadiusServerDrop(clientIPAddr, strconv.Itoa(int(code)))
				return
			}

			// Write to socker
			if _, err = socket.WriteTo(respBuf, addr); err != nil {
				core.GetLogger().Errorf("error sending packet to %s with code %d: %s", addr.String(), code, err)
				core.RecordRadiusServerDrop(clientIPAddr, strconv.Itoa(int(code)))
				return
			}

			core.RecordRadiusServerResponse(clientIPAddr, strconv.Itoa(int(code)))
			core.GetLogger().Debugf("-> Server sent RadiusPacket %s\n", responsePacket)

		}(radiusPacket, radiusClient.Secret, clientAddr)
	}
}
