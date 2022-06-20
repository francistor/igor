package router

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

// Message to be sent for orderly shutdown of the Router
type RouterCloseCommand struct {
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

// The Router handles the lifecycle of peers and routes Diameter requests
// to the appropriate destinations
// It follows the Actor model. All actions take place in the event loop
type Router struct {

	// Configuration instance
	instanceName string

	// Stauts of the Router
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

	// To send commands to the Router
	routerControlChannel chan interface{}

	// To signal that the Router has shut down
	RouterDoneChannel chan struct{}

	// HTTP2 client
	http2Client http.Client
}

// Creates and runs a Router
func NewRouter(instanceName string) *Router {

	router := Router{
		instanceName:         instanceName,
		diameterPeersTable:   make(map[string]DiameterPeerWithStatus),
		peerTableTicker:      time.NewTicker(60 * time.Second),
		peerControlChannel:   make(chan interface{}, 10),
		diameterRequestsChan: make(chan RoutableDiameterRequest, 10),
		routerControlChannel: make(chan interface{}),
		RouterDoneChannel:    make(chan struct{}),
	}

	// Configure client
	transportCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	router.http2Client = http.Client{Timeout: 2 * time.Second, Transport: transportCfg}

	go router.eventLoop()

	return &router
}

// Starts the closing process. It will set in StatusClosing stauts. The
func (router *Router) Close() {
	router.routerControlChannel <- RouterCloseCommand{}
}

// Actor model event loop
func (router *Router) eventLoop() {

	logger := config.GetLogger()

	// Server socket
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", config.GetPolicyConfigInstance(router.instanceName).DiameterServerConf().BindPort))
	if err != nil {
		panic(err)
	}
	// Assign to instance variable
	router.listener = listener

	// Accepter loop
	go func() {
		for {
			logger.Info("diameter server accepting connections")
			connection, err := router.listener.Accept()
			if err != nil {
				if router.status != StatusClosing {
					logger.Info("error accepting connection")
					panic(err)
				}
				// Finish acceptor loop
				return
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			logger.Infof("accepted connection from %s", remoteAddr)

			remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)
			peersConf := config.GetPolicyConfigInstance(router.instanceName).PeersConf()
			if !peersConf.ValidateIncomingAddress("", remoteIPAddr.IP) {
				logger.Infof("invalid peer %s\n", remoteIPAddr)
				connection.Close()
				continue
			}

			// Create peer for the accepted connection and start it
			// The addition to the peers table will be done later
			// after the PeerUp is received and checking that there is not a duplicate
			diampeer.NewPassiveDiameterPeer(router.instanceName, router.peerControlChannel, connection, func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
				return router.RouteDiameterRequest(request, 0)
			})
		}
	}()

	// First pass
	router.updatePeersTable()

routerEventLoop:
	for {
	messageHandler:
		select {

		// Handle lifecycle messages for this Router
		case m := <-router.routerControlChannel:
			switch m.(type) {
			case RouterCloseCommand:
				// Set the status
				router.status = StatusClosing

				// Close the listener
				router.listener.Close()

				// Close all peers that are up
				// TODO: Check that it is no harm to send two SetDown()
				for peer := range router.diameterPeersTable {
					if router.diameterPeersTable[peer].IsUp {
						router.diameterPeersTable[peer].Peer.SetDown()
					}
				}

				// Check if we can exit
				for peer := range router.diameterPeersTable {
					if router.diameterPeersTable[peer].IsUp {
						break messageHandler
					}
				}

				// If here, all peers are not up
				// Signal to the outside
				router.RouterDoneChannel <- struct{}{}
				break routerEventLoop
			}

		case <-router.peerTableTicker.C:
			// Update peers

		// Receive lifecycle messages from managed Peers
		case m := <-router.peerControlChannel:
			switch v := m.(type) {
			case diampeer.PeerUpEvent:
				if peerEntry, found := router.diameterPeersTable[v.DiameterHost]; found {
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
							router.diameterPeersTable[v.DiameterHost] = DiameterPeerWithStatus{Peer: v.Sender, IsEngaged: true, IsUp: true, LastStatusChange: time.Now(), LastError: nil}
							logger.Infof("new peer entry for %s", v.DiameterHost)
						}
					} else {
						// It is the one reporting up. Only change state
						peerEntry.IsEngaged = true
						peerEntry.LastStatusChange = time.Now()
						peerEntry.LastError = nil
						router.diameterPeersTable[v.DiameterHost] = peerEntry
						logger.Infof("updating peer entry for %s", v.DiameterHost)
					}

					// If we are closing the shop, instructs clients to leave
					if router.status == StatusClosing {
						v.Sender.SetDown()
					}
				} else {
					// Peer not configured. There must have been a race condition
					logger.Warnf("unconfigured peer %s. Disengaging", v.DiameterHost)
					v.Sender.SetDown()
				}

				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

			case diampeer.PeerDownEvent:
				// Closing may take time
				logger.Infof("closing %s", v.Sender.PeerConfig.DiameterHost)
				go v.Sender.Close()

				// Look for peer based on pointer identity, not OriginHost identity
				// Mark as disengaged. Ignore if not found (might be unconfigured)
				// or taken over by another peer
				for originHost, existingPeer := range router.diameterPeersTable {
					if existingPeer.Peer == v.Sender {
						existingPeer.IsEngaged = false
						existingPeer.IsUp = false
						existingPeer.LastStatusChange = time.Now()
						existingPeer.LastError = v.Error
						existingPeer.Peer = nil
						router.diameterPeersTable[originHost] = existingPeer
					}
				}

				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

				// Check if we must exit
				if router.status == StatusClosing {
					// Check if we can exit
					for peer := range router.diameterPeersTable {
						if router.diameterPeersTable[peer].IsUp {
							break messageHandler
						}
					}

					// If here, all peers are not up
					// Signal to the outside
					router.RouterDoneChannel <- struct{}{}
					break routerEventLoop
				}
			}

			// Diameter Request message to be routed
		case rdr := <-router.diameterRequestsChan:
			route, err := config.GetPolicyConfigInstance(router.instanceName).RoutingRulesConf().FindDiameterRoute(
				rdr.Message.GetStringAVP("Destination-Realm"),
				rdr.Message.ApplicationName,
				false)

			if err != nil {
				instrumentation.PushDiameterRequestDiscarded("", rdr.Message)
				rdr.RChan <- fmt.Errorf("request not sent: no route found")
				break messageHandler
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
					targetPeer := router.diameterPeersTable[destinationHost]
					if targetPeer.IsEngaged {
						// Route found. Send request asyncronously
						go targetPeer.Peer.DiameterExchangeWithChannel(rdr.Message, rdr.Timeout, rdr.RChan)
						break messageHandler
					}
				}

				// If here, could not find a peer
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
					httpResp, err := router.http2Client.Post(destinationURLs[0], "application/json", bytes.NewReader(jsonRequest))
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
					rdr.RChan <- &diameterAnswer

				}(rdr.RChan, rdr.Message)

			} else {
				panic("bad route, without peers or handlers")
			}
		}
	}
	logger.Infof("finished Peer manager %s ", router.instanceName)
}

// Sends a DiameterMessage and returns a channel for the response or error
// TODO: Make sure that the response channel is closed
func (router *Router) RouteDiameterRequest(request *diamcodec.DiameterMessage, timeout time.Duration) (*diamcodec.DiameterMessage, error) {
	responseChannel := make(chan interface{}, 1)

	routableRequest := RoutableDiameterRequest{
		Message: request,
		RChan:   responseChannel,
		Timeout: timeout,
	}
	router.diameterRequestsChan <- routableRequest

	r := <-routableRequest.RChan
	switch v := r.(type) {
	case error:
		return &diamcodec.DiameterMessage{}, v
	case *diamcodec.DiameterMessage:
		return v, nil
	}
	panic("got an answer that was not error or pointer to diameter message")
}

func (router *Router) RouteDiameterRequestAsync(request *diamcodec.DiameterMessage, timeout time.Duration, handler func(resp *diamcodec.DiameterMessage, e error)) {
	responseChannel := make(chan interface{}, 1)

	routableRequest := RoutableDiameterRequest{
		Message: request,
		RChan:   responseChannel,
		Timeout: timeout,
	}
	router.diameterRequestsChan <- routableRequest

	r := <-routableRequest.RChan
	switch v := r.(type) {
	case error:
		handler(&diamcodec.DiameterMessage{}, v)
	case *diamcodec.DiameterMessage:
		handler(v, nil)
	}
	panic("got an answer that was not error or pointer to diameter message")
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
func (router *Router) updatePeersTable() {

	// Do nothing if we are closing
	if router.status == StatusClosing {
		return
	}

	// Get the current configuration
	diameterPeersConf := config.GetPolicyConfigInstance(router.instanceName).PeersConf()

	// Force non configured peers to disengage
	// The real removal from the table will take place when the PeerDownEvent is received
	for existingDH := range router.diameterPeersTable {
		if _, found := diameterPeersConf[existingDH]; !found {
			peer := router.diameterPeersTable[existingDH]
			// The table will be updated and this peer removed wheh the PeerDown event is received
			peer.Peer.SetDown()
			peer.IsEngaged = false
		}
	}

	// Make sure an entry exists for each configured peer
	for dh := range diameterPeersConf {
		peerConfig := diameterPeersConf[dh]
		_, found := router.diameterPeersTable[diameterPeersConf[dh].DiameterHost]
		if !found {
			if peerConfig.ConnectionPolicy == "active" {
				diamPeer := diampeer.NewActiveDiameterPeer(router.instanceName, router.peerControlChannel, peerConfig, func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
					return router.RouteDiameterRequest(request, 0)
				})
				router.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: diamPeer, IsEngaged: false, IsUp: true, LastStatusChange: time.Now()}
			} else {
				router.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{Peer: nil, IsEngaged: false, IsUp: true, LastStatusChange: time.Now()}
			}
		}
	}

	instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())
}

// Generates the DiameterPeersTableEntry for instrumetation purposes, using the current
// internal table and shuffling the fields as necessary to adjust the contents
func (router *Router) buildPeersStatusTable() instrumentation.DiameterPeersTable {
	peerTable := make([]instrumentation.DiameterPeersTableEntry, 0)

	for diameterHost, peerStatus := range router.diameterPeersTable {
		var ipAddress string = ""
		var connectionPolicy = ""
		if peerStatus.Peer != nil {
			// Take from effective values
			ipAddress = peerStatus.Peer.PeerConfig.IPAddress
			connectionPolicy = peerStatus.Peer.PeerConfig.ConnectionPolicy
		} else {
			// Take from configuration
			diameterPeersConf := config.GetPolicyConfigInstance(router.instanceName).PeersConf()
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
