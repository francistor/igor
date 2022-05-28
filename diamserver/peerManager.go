package diamserver

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"igor/instrumentation"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// Message to be sent for orderly shutdown of the PeerManager
type PeerManagerCloseCommand struct {
}

type DiameterPeerWithStatus struct {
	Peer *diampeer.DiameterPeer
	// True when the PeerUp command has been received
	IsEngaged bool
	// True as soon as the Peer is created, and a PeerDown event must be received to signal that it can be closed
	IsUp             bool
	LastStatusChange time.Time
	LastError        error
}

const (
	StatusOperational = 0
	StatusClosing     = 1
)

// Represents a Diameter Message to be routed, either to a handler
// or to another Diameter Peer
type RoutableDiameterRequest struct {
	Message *diamcodec.DiameterMessage

	// The channel to send the answer or error
	RChan chan interface{}

	// Timeout
	Timeout time.Duration
}

// The DiameterPeerManager handles the lifecycle of peers and routes Diameter requests
// to the appropriate destinations
// It follows the Actor model. All actions take place in the event loop
type DiameterPeerManager struct {

	// Configuration instance
	instanceName string

	// Stauts of the PeerManager
	status int

	// Accepter of incoming connections
	listener net.Listener

	// Holds the Peers Table. Initially emtpy
	// Passive peers with the other party not yet identified will not be here
	diameterPeersTable map[string]DiameterPeerWithStatus

	// Time to check the peer status. Reload configuration and check if new connections
	// need to be established or closed
	peerTableTicker *time.Ticker

	// Passed to the DiameterPeers to receive lifecycle events
	peerControlChannel chan interface{}

	// Exposed to retreive Diameter Requests
	diameterRequestsChan chan RoutableDiameterRequest

	// To send commands to the PeerManager
	managerControlChannel chan interface{}

	// To signal that the PeerManager has shut down
	ManagerDoneChannel chan struct{}

	// HTTP2 client
	http2Client http.Client
}

// Creates and runs a DiameterPeerManager
func NewDiameterPeerManager(instanceName string) *DiameterPeerManager {

	pm := DiameterPeerManager{
		instanceName:          instanceName,
		diameterPeersTable:    make(map[string]DiameterPeerWithStatus),
		peerTableTicker:       time.NewTicker(60 * time.Second),
		peerControlChannel:    make(chan interface{}, 10),
		diameterRequestsChan:  make(chan RoutableDiameterRequest, 10),
		managerControlChannel: make(chan interface{}),
		ManagerDoneChannel:    make(chan struct{}),
	}

	// Configure client
	transportCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	pm.http2Client = http.Client{Timeout: 2 * time.Second, Transport: transportCfg}

	go pm.eventLoop()

	return &pm
}

// Starts the closing process. It will set in StatusClosing stauts. The
func (pm *DiameterPeerManager) Close() {
	pm.managerControlChannel <- PeerManagerCloseCommand{}
}

// Actor model event loop
func (pm *DiameterPeerManager) eventLoop() {

	logger := config.GetConfigInstance(pm.instanceName).IgorLogger

	// Server socket
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", config.GetConfigInstance(pm.instanceName).DiameterServerConf().BindPort))
	if err != nil {
		panic(err)
	}
	// Assign to instance variable
	pm.listener = listener

	// Accepter loop
	go func() {
		for {
			logger.Info("diameter server accepting connections")
			connection, err := pm.listener.Accept()
			if err != nil {
				if pm.status != StatusClosing {
					logger.Info("error accepting connection")
					panic(err)
				}
				// Finish acceptor loop
				return
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
			diampeer.NewPassiveDiameterPeer(pm.instanceName, pm.peerControlChannel, connection, func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
				r := <-pm.RouteDiameterRequest(request, 0)
				switch v := r.(type) {
				case error:
					return &diamcodec.DiameterMessage{}, v
				case *diamcodec.DiameterMessage:
					return v, nil
				}
				panic("got an answer that was not error or pointer to diameter message")
			})
		}
	}()

	// First pass
	pm.updatePeersTable()

peerManagerEventLoop:
	for {
	messageHandler:
		select {

		// Handle lifecycle messages for this PeerManager
		case m := <-pm.managerControlChannel:
			switch m.(type) {
			case PeerManagerCloseCommand:
				// Set the status
				pm.status = StatusClosing

				// Close the listener
				pm.listener.Close()

				// Close all peers that are up
				// TODO: Check that it is no harm to send two SetDown()
				for peer := range pm.diameterPeersTable {
					if pm.diameterPeersTable[peer].IsUp {
						pm.diameterPeersTable[peer].Peer.SetDown()
					}
				}

				// Check if we can exit
				for peer := range pm.diameterPeersTable {
					if pm.diameterPeersTable[peer].IsUp {
						break messageHandler
					}
				}

				// If here, all peers are not up
				// Signal to the outside
				pm.ManagerDoneChannel <- struct{}{}
				break peerManagerEventLoop
			}

		case <-pm.peerTableTicker.C:
			// Update peers

		// Receive lifecycle messages from managed Peers
		case m := <-pm.peerControlChannel:
			switch v := m.(type) {
			case diampeer.PeerUpEvent:
				if peerEntry, found := pm.diameterPeersTable[v.DiameterHost]; found {
					// Entry found for this peer (normal)
					if peerEntry.Peer != v.Sender {
						// And is not the one reporting up
						if peerEntry.Peer != nil && peerEntry.IsEngaged {
							// The existing peer wins. Disengage the newly received one
							v.Sender.SetDown()
							logger.Infof("keeping already engaged peer entry for %s", v.DiameterHost)
						} else {
							// The new peer wins. Disengage the existing one if there is one
							if peerEntry.Peer != nil {
								peerEntry.Peer.SetDown()
								logger.Infof("closing not engaged peer entry for %s", v.DiameterHost)
							}
							// Update the peers table
							pm.diameterPeersTable[v.DiameterHost] = DiameterPeerWithStatus{Peer: v.Sender, IsEngaged: true, IsUp: true, LastStatusChange: time.Now(), LastError: nil}
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

					// If we are closing the shop, instructs clients to leave
					if pm.status == StatusClosing {
						v.Sender.SetDown()
					}
				} else {
					// Peer not configured. There must have been a race condition
					logger.Warnf("unconfigured peer %s. Disengaging", v.DiameterHost)
					v.Sender.SetDown()
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
						existingPeer.IsUp = false
						existingPeer.LastStatusChange = time.Now()
						existingPeer.LastError = v.Error
						existingPeer.Peer = nil
						pm.diameterPeersTable[originHost] = existingPeer
					}
				}

				instrumentation.PushDiameterPeersStatus(pm.instanceName, pm.buildPeersStatusTable())

				// Check if we must exit
				if pm.status == StatusClosing {
					// Check if we can exit
					for peer := range pm.diameterPeersTable {
						if pm.diameterPeersTable[peer].IsUp {
							break messageHandler
						}
					}

					// If here, all peers are not up
					// Signal to the outside
					pm.ManagerDoneChannel <- struct{}{}
					break peerManagerEventLoop
				}
			}

			// Diameter Request message to be routed
		case rdr := <-pm.diameterRequestsChan:
			route, err := config.GetConfigInstance(pm.instanceName).RoutingRulesConf().FindDiameterRoute(
				rdr.Message.GetStringAVP("Destination-Realm"),
				rdr.Message.ApplicationName,
				false)

			if err != nil {
				instrumentation.PushDiameterRequestDiscarded("", rdr.Message)
				rdr.RChan <- fmt.Errorf("request not sent: no route found")
				break
			}

			if len(route.Peers) > 0 {
				// Route to destination peer
				// If policy is "random", shuffle the destination-hosts
				var peers []string
				peers = append(peers, route.Peers...)

				if route.Policy == "random" {
					rand.Seed(time.Now().UnixNano())
					rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
				}

				for _, destinationHost := range peers {
					targetPeer := pm.diameterPeersTable[destinationHost]
					fmt.Println(targetPeer.IsEngaged)
					if targetPeer.IsEngaged {
						// Route found. Send request asyncronously
						go targetPeer.Peer.DiameterExchangeWithChannel(rdr.Message, rdr.Timeout, rdr.RChan)
						break messageHandler
					}
				}

				// If here, could not find a route
				instrumentation.PushDiameterRequestDiscarded("", rdr.Message)
				rdr.RChan <- fmt.Errorf("resquest not sent: no engaged peer")
				close(rdr.RChan)

			} else if len(route.Handlers) > 0 {
				// Handle locally

				// For handlers there is not such a thing as fixed policy
				var destinationURLs []string
				destinationURLs = append(destinationURLs, route.Handlers...)
				rand.Seed(time.Now().UnixNano())
				rand.Shuffle(len(destinationURLs), func(i, j int) { destinationURLs[i], destinationURLs[j] = destinationURLs[j], destinationURLs[i] })

				go func(respChan chan interface{}, diameterRequest *diamcodec.DiameterMessage) {

					// Make sure the response channel is closed
					defer close(respChan)

					// Serialize the message
					jsonRequest, err := json.Marshal(diameterRequest)
					if err != nil {
						logger.Errorf("unable to marshal message to json %s", err)
						rdr.RChan <- fmt.Errorf("unable to marshal message to json %s", err)
						return
					}

					// Send the request to the Handler
					httpResp, err := pm.http2Client.Post(destinationURLs[0], "application/json", bytes.NewReader(jsonRequest))
					if err != nil {
						logger.Errorf("handler %s error %s", destinationURLs[0], err)
						rdr.RChan <- err
						return
					}
					defer httpResp.Body.Close()

					if httpResp.StatusCode != 200 {
						logger.Errorf("handler %s returned status code %d", httpResp.StatusCode)
						rdr.RChan <- fmt.Errorf("handler %s returned status code %d", destinationURLs[0], httpResp.StatusCode)
						return
					}

					jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
					if err != nil {
						logger.Errorf("error reading response from %s %s", destinationURLs[0], err)
						rdr.RChan <- err
						return
					}

					// Unserialize to Diameter Message
					var diameterAnswer diamcodec.DiameterMessage
					err = json.Unmarshal(jsonAnswer, &diameterAnswer)
					if err != nil {
						logger.Errorf("error unmarshaling response from %s %s", destinationURLs[0], err)
						rdr.RChan <- err
						return
					}

					// All good. Answer
					rdr.RChan <- diameterAnswer

				}(rdr.RChan, rdr.Message)

			} else {
				panic("bad route, without peers or handlers")
			}
		}
	}
	logger.Infof("finished Peer manager %s ", pm.instanceName)
}

// Sends a DiameterMessage and returns a channel for the response or error
// TODO: Make sure that the response channel is closed
func (pm *DiameterPeerManager) RouteDiameterRequest(request *diamcodec.DiameterMessage, timeout time.Duration) chan interface{} {
	responseChannel := make(chan interface{})

	routableRequest := RoutableDiameterRequest{
		Message: request,
		RChan:   responseChannel,
		Timeout: timeout,
	}
	pm.diameterRequestsChan <- routableRequest

	return responseChannel
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
func (pm *DiameterPeerManager) updatePeersTable() {

	// Do nothing if we are closing
	if pm.status == StatusClosing {
		return
	}

	// Get the current configuration
	diameterPeersConf := config.GetConfigInstance(pm.instanceName).PeersConf()

	// Force non configured peers to disengage
	// The real removal from the table will take place when the PeerDownEvent is received
	for existingDH := range pm.diameterPeersTable {
		if _, found := diameterPeersConf[existingDH]; !found {
			peer := pm.diameterPeersTable[existingDH]
			// The table will be updated and this peer removed wheh the PeerDown event is received
			peer.Peer.SetDown()
			peer.IsEngaged = false
		}
	}

	// Make sure an entry exists for each configured peer
	for dh := range diameterPeersConf {
		peerConfig := diameterPeersConf[dh]
		_, found := pm.diameterPeersTable[diameterPeersConf[dh].DiameterHost]
		if !found {
			if peerConfig.ConnectionPolicy == "active" {
				diamPeer := diampeer.NewActiveDiameterPeer(pm.instanceName, pm.peerControlChannel, peerConfig, func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
					r := <-pm.RouteDiameterRequest(request, 0)
					switch v := r.(type) {
					case error:
						return &diamcodec.DiameterMessage{}, v
					case *diamcodec.DiameterMessage:
						return v, nil
					}
					panic("got an answer that was not error or pointer to diameter message")
				})
				pm.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: diamPeer, IsEngaged: false, IsUp: true, LastStatusChange: time.Now()}
			} else {
				pm.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: nil, IsEngaged: false, IsUp: true, LastStatusChange: time.Now()}
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

func _ManagerMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDefaultDiameterAnswer(request)
	return &answer, nil
}
