package radiusserver

import (
	"fmt"
	"igor/config"
	"igor/instrumentation"
	"igor/radiuscodec"
	"net"
	"sync/atomic"
)

// Valid statuses
const (
	StatusTerminated = 1
)

// Type for functions that handle the radius requests received
type RadiusPacketHandler func(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error)

// Implements a radius server socket
// Validates incoming messages, sends them to the router for processing and replies back
// with the responses
type RadiusServer struct {

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// Handler function for incoming packets
	handler RadiusPacketHandler

	// The UDP socket
	socket net.PacketConn

	// Status. Initially 0 and 1 (StatusClosing) if we are shutting down
	status int32
}

// Creates a Radius Server
func NewRadiusServer(ci *config.PolicyConfigurationManager, bindAddress string, bindPort int, handler RadiusPacketHandler) *RadiusServer {

	// Create the server socket
	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindAddress, bindPort))
	if err != nil {
		panic(fmt.Sprintf("could not create listen socket in %s:%d : %s", bindAddress, bindPort, err))
	} else {
		config.GetLogger().Infof("RADIUS server listening in %s:%d", bindAddress, bindPort)
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
				config.GetLogger().Infof("finished radius server socket %s", socket.LocalAddr().String())
				return
			} else {
				// Some other error
				panic(err)
			}
		}

		// Verify client and get shared secret
		clientIPAddr := clientAddr.(*net.UDPAddr).IP.String()
		radiusClient, found := rs.ci.RadiusClientsConf()[clientIPAddr]

		if !found {
			config.GetLogger().Debugf("message from unknown client %s", clientIPAddr)
			continue
		}

		// Decode the packet
		radiusPacket, err := radiuscodec.RadiusPacketFromBytes((reqBuf[:packetSize]), radiusClient.Secret)
		if err != nil {
			config.GetLogger().Errorf("error decoding packet %s", err)
			continue
		}

		instrumentation.PushRadiusServerRequest(clientIPAddr, string(radiusPacket.Code))
		config.GetLogger().Debugf("<- Server received RadiusPacket %s\n", radiusPacket)

		// Wait for response
		go func(radiusPacket *radiuscodec.RadiusPacket, secret string, addr net.Addr) {

			code := radiusPacket.Code

			response, err := rs.handler(radiusPacket)

			if err != nil {
				config.GetLogger().Errorf("discarding packet for %s with code %d: %s", addr.String(), radiusPacket.Code, err)
				instrumentation.PushRadiusServerDrop(clientIPAddr, string(radiusPacket.Code))
				return
			}

			respBuf, err := response.ToBytes(secret, radiusPacket.Identifier)
			if err != nil {
				config.GetLogger().Errorf("error serializing packet for %s with code %d: %s", addr.String(), code, err)
				instrumentation.PushRadiusServerDrop(clientIPAddr, string(code))
				return
			}
			if _, err = socket.WriteTo(respBuf, addr); err != nil {
				config.GetLogger().Errorf("error sending packet to %s with code %d: %s", addr.String(), code, err)
				instrumentation.PushRadiusServerDrop(clientIPAddr, string(code))
				return
			}

			instrumentation.PushRadiusServerResponse(clientIPAddr, string(code))
			config.GetLogger().Debugf("-> Server sent RadiusPacket %s\n", response)

		}(radiusPacket, radiusClient.Secret, clientAddr)
	}
}
