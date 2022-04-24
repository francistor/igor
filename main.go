package main

import (
	"flag"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"net"
)

const (
	statusRunning = 0
	statusClosing = 1
)

func main() {

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	config.InitConfigurationInstance(*bootPtr, *instancePtr)

	// Get logger
	igorLogger := config.GetConfigInstance(*instancePtr).IgorLogger

	var routerStatus = statusRunning

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", config.GetConfigInstance(*instancePtr).DiameterServerConf().BindPort))
	if err != nil {
		panic(err)
	}

	var routerControlChannel = make(chan interface{})

	diameterPeersConf := config.GetConfigInstance(*instancePtr).PeersConf()

	// Holds the current Peers Table. Initially emtpy
	// Passive peers with the other party not yet identified will not be here
	diamPeersTable := make(map[string]*diampeer.DiameterPeer)

	// Accepter loop
	go func() {
		for {
			igorLogger.Info("diameter server accepting connections")
			connection, err := listener.Accept()
			if err != nil {
				panic(err)
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			igorLogger.Infof("accepted connection from %s", remoteAddr)

			remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)
			if !diameterPeersConf.ValidateIncomingAddress("", remoteIPAddr.IP) {
				igorLogger.Infof("invalid peer %s\n", remoteIPAddr)
				connection.Close()
				continue
			}

			// Create peer for the accepted connection and start it
			// The addition to the peers table will be done later
			diampeer.NewPassiveDiameterPeer(*instancePtr, routerControlChannel, connection, MyMessageHandler)
		}
	}()

	// Initialize
	diamPeersTable = updatePeersTable(*instancePtr, routerControlChannel, diamPeersTable, diameterPeersConf)

	// Peers Lifecycle
	for {
		msg := <-routerControlChannel
		switch v := msg.(type) {

		// New Peer is engaged
		case diampeer.PeerUpEvent:

			igorLogger.Debugf("peerup event from %s", v.Sender.PeerConfig.DiameterHost)

			if existingPeer, found := diamPeersTable[v.Sender.PeerConfig.DiameterHost]; found {
				if existingPeer != v.Sender {
					// Peer already exists. Close the new peer
					existingPeer.Close()
				} // Else do nothing. This is an active peer reporting that it is Engaged
			} else {
				// Register the new peer
				diamPeersTable[v.Sender.PeerConfig.DiameterHost] = v.Sender
			}

		// Peer is down
		case diampeer.PeerDownEvent:

			igorLogger.Debugf("peerup event from %s", v.Sender.PeerConfig.DiameterHost)

			// Remove from peers table
			delete(diamPeersTable, v.Sender.PeerConfig.DiameterHost)

			// If in normal operation, update the peers
			if routerStatus == statusRunning {
				diamPeersTable = updatePeersTable(*instancePtr, routerControlChannel, diamPeersTable, diameterPeersConf)
			} else {
				// We are closing the shop
				if len(diamPeersTable) == 0 {
					break
				}
			}
		}
	}

	// close(routerControlChannel)
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
func updatePeersTable(instanceName string, controlCh chan interface{}, peersTable map[string]*diampeer.DiameterPeer, diameterPeersConf config.DiameterPeers) map[string]*diampeer.DiameterPeer {

	// Close the connections for now not configured peers
	for existingDH := range peersTable {
		if _, found := diameterPeersConf[existingDH]; !found {
			peer := peersTable[existingDH]
			// The table will be updated and this peer removed wheh the PeerDown event is received
			peer.Close()
		}
	}

	// Make sure a DiameterPeer exists for each active peer
	for dh := range diameterPeersConf {
		peerConfig := diameterPeersConf[dh]
		if peerConfig.ConnectionPolicy == "active" {
			_, found := peersTable[diameterPeersConf[dh].DiameterHost]
			if !found {
				diamPeer := diampeer.NewActiveDiameterPeer(instanceName, controlCh, peerConfig, MyMessageHandler)
				peersTable[peerConfig.DiameterHost] = diamPeer
			}
		}
	}

	return peersTable
}

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	return &answer, nil
}
