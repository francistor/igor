package radiusserver

import (
	"context"
	"fmt"
	"igor/config"
	"igor/instrumentation"
	"igor/radiuscodec"
	"igor/router"
	"net"
)

// Implements a radius server socket
// Validates incoming messages, sends them to the router for processing and sends the responses
type RadiusServer struct {

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// The channel to send the messages to
	routerChannel chan router.RoutableRadiusRequest

	// Context for cancellation
	context context.Context
}

func NewRadiusServer(ctx context.Context, ci *config.PolicyConfigurationManager, bindIPAddress string, bindPort int, ch chan router.RoutableRadiusRequest) *RadiusServer {

	radiusServer := RadiusServer{
		ci:            ci,
		routerChannel: ch,
		context:       ctx,
	}

	socket, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bindIPAddress, bindPort))
	if err != nil {
		panic(fmt.Sprintf("could not create listen socket in %s:%d : %s", bindIPAddress, bindPort, err))
	}

	// Start receiving packets
	go radiusServer.eventLoop(socket)

	return &radiusServer
}

func (rs *RadiusServer) eventLoop(socket net.PacketConn) {

	// Close socket and exit whent the context is done
	go func() {
		<-rs.context.Done()

		// Will generate an error in the loop, and the eventLoop will return
		socket.Close()
	}()

	// Single buffer where all incoming packets are read
	// According to RFC 2865, the maximum packet size is 4096
	reqBuf := make([]byte, 4096)

	for {
		packetSize, clientAddr, err := socket.ReadFrom(reqBuf)
		if err != nil {
			// Check here if the error is due to the socket being closed
			if rs.context.Err() != nil {
				// The context was cancelled
				config.GetLogger().Infof("finished radius server socket %s", socket.LocalAddr().String())
				return
			} else {
				// Some other error
				panic(err)
			}
		}

		// Verify client and get secret
		clientIPAddr := clientAddr.(*net.UDPAddr).IP.String()
		radiusClient, found := rs.ci.RadiusClientsConf()[clientIPAddr]

		if !found {
			config.GetLogger().Debugf("message from unknown client %s", clientIPAddr)
			break
		}

		// Decode the packet
		radiusPacket, err := radiuscodec.RadiusPacketFromBytes((reqBuf[:packetSize]), radiusClient.Secret)
		if err != nil {
			config.GetLogger().Errorf("error decoding packet %s", err)
		}

		// Channel for answer
		rChan := make(chan interface{})

		// Send the request to the router
		routableReq := router.RoutableRadiusRequest{
			Secret:       radiusClient.Secret,
			RadiusPacket: radiusPacket,
			RChan:        rChan,
		}
		rs.routerChannel <- routableReq
		instrumentation.PushRadiusServerRequest(clientIPAddr, string(radiusPacket.Code))
		config.GetLogger().Debugf("<- Server received RadiusPacket %s\n", radiusPacket)

		// Wait for response
		go func(secret string, addr net.Addr, code byte) {
			clientIPAddr := clientAddr.(*net.UDPAddr).IP.String()
			r := <-rChan

			switch v := r.(type) {
			case error:
				config.GetLogger().Errorf("discarding packet for %s with code %d: %s", addr.String(), code, err)
				instrumentation.PushRadiusServerDrop(clientIPAddr, string(code))

			case *radiuscodec.RadiusPacket:

				respBuf, err := v.ToBytes(secret, v.Identifier)
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
				config.GetLogger().Debugf("-> Server sent RadiusPacket %s\n", v)

			}
		}(radiusClient.Secret, clientAddr, radiusPacket.Code)
	}
}
