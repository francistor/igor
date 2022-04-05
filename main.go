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
			diampeer.NewPassiveDiameterPeer(routerInputChannel, connection, MyMessageHandler)
		}
	}()

	// Create the DiameterPeer objects after some time for accepter loop to be executed
	time.Sleep(2 * time.Second)

	// Initially emtpy
	diamPeers := make(map[string]diampeer.DiameterPeer)
	// Will create a DiameterPeer per active Peer
	diamPeers = updateDiameterPeers(routerInputChannel, diamPeers, diameterPeersConf)

	// Use peer for superserver.igor
	superserverPeer := diamPeers["superserver.igor"]

	// Wait until received connected event sent by active peer and then send message
	_, ok := (<-routerInputChannel).(diampeer.PeerUpEvent)
	if ok {
		diameterMessage, error := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
		if error != nil {
			panic(error)
		}
		diameterMessage.Add("User-Name", "Perico")
		answer, err := superserverPeer.DiameterRequest(&diameterMessage, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("GOT ANSWER", answer)
		}
	} else {
		fmt.Println("did not get a peer up event")
	}

	// Close active peer that sent the message
	time.Sleep(1 * time.Second)
	superserverPeer.Close()

	fmt.Println("waiting to terminate...")
	time.Sleep(5 * time.Second)
	fmt.Println("done.")

	close(routerInputChannel)
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
func updateDiameterPeers(c chan interface{}, diamPeers map[string]diampeer.DiameterPeer, diameterPeers config.DiameterPeers) map[string]diampeer.DiameterPeer {

	// TODO: Close the connections for now not configured peers

	// Make sure a DiameterPeer exists for each active peer
	for dh := range diameterPeers {
		peerConfig := diameterPeers[dh]
		if peerConfig.ConnectionPolicy == "active" {
			_, found := diamPeers[diameterPeers[dh].DiameterHost]
			if !found {
				diamPeer := diampeer.NewActiveDiameterPeer(c, peerConfig, MyMessageHandler)
				diamPeers[peerConfig.DiameterHost] = diamPeer
			}
		}
	}

	return diamPeers
}

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	return &answer, nil
}
