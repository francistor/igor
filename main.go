package main

import (
	"flag"
	"fmt"
	"igor/config"
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

	// Create the PeerSocket objects
	peerSockets := make([]diampeer.PeerSocket, 0)
	diameterPeers, _ := config.GetDiameterPeers()
	for i, _ := range diameterPeers {
		var peerSocket = diampeer.PeerSocket{PeerConfig: diameterPeers[i]}
		peerSockets = append(peerSockets, peerSocket)
	}
	fmt.Println(diameterPeers)

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":3868")
	if err != nil {
		panic(err)
	}

	// Accepter loop
	go func() {
		for {
			fmt.Println("accepting connections ...")
			connection, err := listener.Accept()
			if err != nil {
				panic(err)
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			fmt.Printf("accepted connection from %s\n", remoteAddr)
			// Look for peer with this address
			for i := range peerSockets {
				if peerSockets[i].PeerConfig.IPAddress == remoteAddr {
					peerSockets[i].SetConnection(connection)
					go peerSockets[i].ReceiveLoop()
					break
				}
			}
		}
	}()

	// Start Peer active connections
	time.Sleep(2 * time.Second)
	for i, _ := range peerSockets {
		if peerSockets[i].PeerConfig.ConnectionPolicy == "active" {
			fmt.Println("Connecting peer " + peerSockets[i].PeerConfig.DiameterHost)
			err := peerSockets[i].Connect()
			if err != nil {
				fmt.Println(err)
			} else {
				go peerSockets[i].ReceiveLoop()
				peerSockets[i].SendMessage([]byte{1, 2, 3})
			}
		}
	}

	fmt.Println("waiting ...")
	time.Sleep(10 * time.Second)
	fmt.Println("done.")

}
