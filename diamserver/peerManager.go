package diamserver

import (
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"igor/instrumentation"
	"net"
	"time"
)

// Represents a Diameter Message to be routed, either to a handler
// or to another Diameter Peer
type RoutableDiameterRequest struct {
	Message *diamcodec.DiameterMessage

	// The channel to send the answer or error
	RC *chan interface{}
}

type DiameterPeerWithStatus struct {
	Peer             *diampeer.DiameterPeer
	IsEngaged        bool
	LastStatusChange time.Time
	LastError        error
}

// The DiameterPeerManager handles the lifecycle of peers and routes Diameter requests
// to the appropriate destinations
// It follows the Actor model. All actions take place in the event loop
type DiameterPeerManager struct {

	// Configuration instance
	instanceName string

	// Holds the Peers Table. Initially emtpy
	// Passive peers with the other party not yet identified will not be here
	diameterPeersTable map[string]DiameterPeerWithStatus

	// Time to check the peer status. Reload configuration and check if new connections
	// need to be established or closed
	peerTableTicker *time.Ticker

	// Passed to the DiameterPeers to receive lifecycle events
	controlChannel chan interface{}

	// Exposed to retreive Diameter Requests
	DRChann chan RoutableDiameterRequest

	// Message handler
	handler diampeer.MessageHandler
}

// Creates and runs a DiameterPeerManager
func NewDiameterPeerManager(instanceName string) *DiameterPeerManager {

	pm := DiameterPeerManager{
		instanceName:       instanceName,
		diameterPeersTable: make(map[string]DiameterPeerWithStatus),
		peerTableTicker:    time.NewTicker(10 * time.Second),
		controlChannel:     make(chan interface{}, 10),
		handler:            MyMessageHandler,
	}

	go pm.eventLoop()

	return &pm
}

func (pm *DiameterPeerManager) eventLoop() {

	logger := config.GetConfigInstance(pm.instanceName).IgorLogger

	// Server socket
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", config.GetConfigInstance(pm.instanceName).DiameterServerConf().BindPort))
	if err != nil {
		panic(err)
	}

	// Accepter loop
	go func() {
		for {
			logger.Info("diameter server accepting connections")
			connection, err := listener.Accept()
			if err != nil {
				panic(err)
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			logger.Infof("accepted connection from %s", remoteAddr)

			remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)
			peersConf := config.GetConfigInstance(pm.instanceName).PeersConf()
			if !peersConf.ValidateIncomingAddress("", remoteIPAddr.IP) {
				logger.Infof("invalid peer %s\n", remoteIPAddr)
				connection.Close()
				continue
			}

			// Create peer for the accepted connection and start it
			// The addition to the peers table will be done later
			// after the PeerUp is received and checking that there is not a duplicate
			diampeer.NewPassiveDiameterPeer(pm.instanceName, pm.controlChannel, connection, MyMessageHandler)
		}
	}()

	// First pass
	pm.updatePeersTable()

	for {
		select {
		case <-pm.peerTableTicker.C:

		case m := <-pm.controlChannel:
			switch v := m.(type) {
			case diampeer.PeerUpEvent:
				if peerEntry, found := pm.diameterPeersTable[v.DiameterHost]; found {
					// Entry found for this peer (normal)
					if peerEntry.Peer != v.Sender {
						// And is not the one reporting up
						if peerEntry.Peer != nil && peerEntry.IsEngaged {
							// The existing peer wins. Disengage the newly received one
							v.Sender.Disengage()
							logger.Infof("keeping already engaged peer entry for %s", v.DiameterHost)
						} else {
							// The new peer wins. Disengage the existing one if there is one
							if peerEntry.Peer != nil {
								peerEntry.Peer.Disengage()
								logger.Infof("closing not engaged peer entry for %s", v.DiameterHost)
							}
							// Update the peers table
							pm.diameterPeersTable[v.DiameterHost] = DiameterPeerWithStatus{Peer: v.Sender, IsEngaged: true, LastStatusChange: time.Now(), LastError: nil}
							logger.Infof("new peer entry for %s", v.DiameterHost)
						}
					} else {
						// It is the one reporting up. Only change state
						peerEntry.IsEngaged = true
						peerEntry.LastStatusChange = time.Now()
						peerEntry.LastError = nil
						pm.diameterPeersTable[v.DiameterHost] = peerEntry
						logger.Infof("updating peer entry for %s", v.DiameterHost)
					}
				} else {
					// Peer not configured. There must have been a race condition
					logger.Warnf("unconfigured peer %s. Disengaging", v.DiameterHost)
					v.Sender.Disengage()
				}

				instrumentation.PushDiameterPeersStatus(pm.instanceName, pm.buildPeersStatusTable())

			case diampeer.PeerDownEvent:
				// Closing may take time
				logger.Infof("closing %s", v.Sender.PeerConfig.DiameterHost)
				go v.Sender.Close()

				// Look for peer based on pointer identity, not OriginHost identity
				// Mark as disengaged. Ignore if not found (might be unconfigured)
				// or taken over by another peer
				for originHost, existingPeer := range pm.diameterPeersTable {
					if existingPeer.Peer == v.Sender {
						existingPeer.IsEngaged = false
						existingPeer.LastStatusChange = time.Now()
						existingPeer.LastError = v.Error
						existingPeer.Peer = nil
						pm.diameterPeersTable[originHost] = existingPeer
					}
				}

				instrumentation.PushDiameterPeersStatus(pm.instanceName, pm.buildPeersStatusTable())
			}

		case <-pm.DRChann:

		}
	}
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
func (pm *DiameterPeerManager) updatePeersTable() {

	// Get the current configuration
	diameterPeersConf := config.GetConfigInstance(pm.instanceName).PeersConf()

	// Force non configured peers to disengage
	// The real removal from the table will take place when the PeerDownEvent is received
	for existingDH := range pm.diameterPeersTable {
		if _, found := diameterPeersConf[existingDH]; !found {
			peer := pm.diameterPeersTable[existingDH]
			// The table will be updated and this peer removed wheh the PeerDown event is received
			peer.Peer.Disengage()
			peer.IsEngaged = false
		}
	}

	// Make sure an entry exists for each configured peer
	for dh := range diameterPeersConf {
		peerConfig := diameterPeersConf[dh]
		_, found := pm.diameterPeersTable[diameterPeersConf[dh].DiameterHost]
		if !found {
			if peerConfig.ConnectionPolicy == "active" {
				diamPeer := diampeer.NewActiveDiameterPeer(pm.instanceName, pm.controlChannel, peerConfig, pm.handler)
				pm.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: diamPeer, IsEngaged: false, LastStatusChange: time.Now()}
			} else {
				pm.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: nil, IsEngaged: false, LastStatusChange: time.Now()}
			}
		}
	}

	instrumentation.PushDiameterPeersStatus(pm.instanceName, pm.buildPeersStatusTable())
}

// Generates the DiameterPeersTableEntry for instrumetation purposes, using the current
// internal table and shuffling the fields as necessary to adjust the contents
func (pm *DiameterPeerManager) buildPeersStatusTable() instrumentation.DiameterPeersTable {
	peerTable := make([]instrumentation.DiameterPeersTableEntry, 0)

	for diameterHost, peerStatus := range pm.diameterPeersTable {
		var ipAddress string = ""
		var connectionPolicy = ""
		if peerStatus.Peer != nil {
			// Take from effective values
			ipAddress = peerStatus.Peer.PeerConfig.IPAddress
			connectionPolicy = peerStatus.Peer.PeerConfig.ConnectionPolicy
		} else {
			// Take from configuration
			diameterPeersConf := config.GetConfigInstance(pm.instanceName).PeersConf()
			peerConfig := diameterPeersConf[diameterHost]
			ipAddress = peerConfig.IPAddress
			connectionPolicy = peerConfig.ConnectionPolicy
		}
		instrumentationEntry := instrumentation.DiameterPeersTableEntry{
			DiameterHost:     diameterHost,
			IPAddress:        ipAddress,
			ConnectionPolicy: connectionPolicy,
			IsEngaged:        peerStatus.IsEngaged,
			LastStatusChange: peerStatus.LastStatusChange,
			LastError:        peerStatus.LastError,
		}
		peerTable = append(peerTable, instrumentationEntry)
	}

	return peerTable
}

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDefaultDiameterAnswer(request)
	return &answer, nil
}
