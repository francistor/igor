package radiusserver

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/francistor/igor/core"
)

// Valid statuses
const (
	StatusTerminated = 1
)

// Implements a radius server socket
// Validates incoming messages, sends them to the router for processing and replies back
// with the responses
type RadiusServer struct {

	// Configuration instance object
	ci *core.PolicyConfigurationManager

	// Handler function for incoming packets
	// handler RadiusPacketHandler
	handler core.RadiusPacketHandler

	// The UDP socket
	socket net.PacketConn

	// Status. Initially 0 and 1 (StatusTerminated) if we are shutting down
	status int32
}

// Creates a Radius Server
func NewRadiusServer(ci *core.PolicyConfigurationManager, bindAddress string, bindPort int, handler core.RadiusPacketHandler) *RadiusServer {

	// Create the server socket
	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindAddress, bindPort))
	if err != nil {
		panic(fmt.Sprintf("could not create listen socket in %s:%d : %s", bindAddress, bindPort, err))
	} else {
		core.GetLogger().Infof("RADIUS server listening in %s:%d", bindAddress, bindPort)
	}

	radiusServer := RadiusServer{
		ci:      ci,
		handler: handler,
		socket:  socket,
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
				core.GetLogger().Infof("finished radius server socket %s", socket.LocalAddr().String())
				return
			} else {
				// Some other error
				panic(err)
			}
		}

		clientIP := clientAddr.(*net.UDPAddr).IP
		clientIPAddr := clientIP.String()
		radiusClient, err := rs.ci.RadiusClients().FindRadiusClient(clientIP)
		if err != nil {
			core.GetLogger().Warnf("message from unknown client %s", clientIPAddr)
			continue
		}

		// Decode the packet
		core.GetLogger().Debugf("received packet: %v", reqBuf[:packetSize])
		radiusPacket, err := core.RadiusPacketFromBytes((reqBuf[:packetSize]), radiusClient.Secret)
		if err != nil {
			core.GetLogger().Errorf("error decoding packet %s\n", err)
			continue
		}

		// Validate the packet
		if radiusPacket.Code != core.ACCESS_REQUEST {
			if !core.ValidateRequestAuthenticator(reqBuf[:packetSize], radiusClient.Secret) {
				core.PushRadiusServerDrop(clientIPAddr, string(radiusPacket.Code))
				core.GetLogger().Warnf("invalid request packet %s\n", radiusPacket)
				continue
			}
		}

		core.PushRadiusServerRequest(clientIPAddr, string(radiusPacket.Code))
		core.GetLogger().Debugf("<- Server received RadiusPacket %s\n", radiusPacket)

		// Wait for response
		go func(radiusPacket *core.RadiusPacket, secret string, addr net.Addr) {

			code := radiusPacket.Code

			response, err := rs.handler(radiusPacket)

			if err != nil {
				core.GetLogger().Errorf("discarding packet for %s with code %d: %s", addr.String(), radiusPacket.Code, err)
				core.PushRadiusServerDrop(clientIPAddr, string(code))
				return
			}

			respBuf, err := response.ToBytes(secret, radiusPacket.Identifier)
			if err != nil {
				core.GetLogger().Errorf("error serializing packet for %s with code %d: %s", addr.String(), code, err)
				core.PushRadiusServerDrop(clientIPAddr, string(code))
				return
			}
			if _, err = socket.WriteTo(respBuf, addr); err != nil {
				core.GetLogger().Errorf("error sending packet to %s with code %d: %s", addr.String(), code, err)
				core.PushRadiusServerDrop(clientIPAddr, string(code))
				return
			}

			core.PushRadiusServerResponse(clientIPAddr, string(code))
			core.GetLogger().Debugf("-> Server sent RadiusPacket %s\n", response)

		}(radiusPacket, radiusClient.Secret, clientAddr)
	}
}
