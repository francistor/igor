package main

import (
	"flag"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"net"
	"time"
)

func main() {

	// Initialize logger
	config.SetupLogger()

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	config.Config.Init(*bootPtr, *instancePtr)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		panic(err)
	}

	var routerInputChannel = make(chan interface{})

	diameterPeersConf := config.PeersConf()

	// Accepter loop
	go func() {
		for {
			config.IgorLogger.Info("diameter server accepting connections")
			connection, err := listener.Accept()
			if err != nil {
				panic(err)
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			config.IgorLogger.Infof("accepted connection from %s", remoteAddr)

			remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)
			if !diameterPeersConf.ValidateIncomingAddress(remoteIPAddr.IP) {
				fmt.Printf("invalid peer %s\n", remoteIPAddr)
			}

			// Create peer for the accepted connection and start it
			diampeer.NewPassivePeerSocket(routerInputChannel, connection)
		}
	}()

	// Create the PeerSocket objects after some time for accepter loop to be executed
	time.Sleep(2 * time.Second)
	// Initially emtpy
	peerSockets := make(map[string]diampeer.PeerSocket)
	// Update
	peerSockets = updatePeerSockets(routerInputChannel, peerSockets, diameterPeersConf)

	// Use peer for superserver.igor
	superserverPeer := peerSockets["superserver.igor"]

	// Wait until received connected event and then send message
	_, ok := (<-routerInputChannel).(diampeer.SocketConnectedEvent)
	if ok {
		diameterMessage, error := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
		if error != nil {
			panic(error)
		}
		diameterMessage.Add("User-Name", "Perico")
		messageBytes, _ := diameterMessage.MarshalBinary()
		superserverPeer.InputChannel <- messageBytes
	} else {
		fmt.Println("peersocket error")
	}

	// Close peer
	time.Sleep(1 * time.Second)
	superserverPeer.InputChannel <- diampeer.SocketCloseCommand{}

	// Wait for down event
	fmt.Println("first message", <-routerInputChannel)
	fmt.Println("second message", <-routerInputChannel)

	fmt.Println("waiting ...")
	time.Sleep(5 * time.Second)
	fmt.Println("done.")

	close(routerInputChannel)

}

// Takes the current map of peerSockets and generates a new one based on the current configuration
func updatePeerSockets(c chan interface{}, peerSockets map[string]diampeer.PeerSocket, diameterPeers config.DiameterPeers) map[string]diampeer.PeerSocket {

	// TODO: Close the connections for now not configured peers

	// Make sure a peerSocket exists for each active peer
	for dh := range diameterPeers {
		peerConfig := diameterPeers[dh]
		if peerConfig.ConnectionPolicy == "active" {
			_, found := peerSockets[diameterPeers[dh].DiameterHost]
			if !found {
				peerSocket := diampeer.NewActivePeerSocket(c, 3000, peerConfig.IPAddress, peerConfig.Port)
				peerSockets[peerConfig.DiameterHost] = peerSocket
			}
		}
	}

	return peerSockets
}
