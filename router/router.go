package router

import (
	"crypto/tls"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"igor/diampeer"
	"igor/httphandler"
	"igor/instrumentation"
	"math/rand"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

// Message to be sent for orderly shutdown of the Router
type RouterCloseCommand struct {
}

// The Diameter Peer table is a map of this kind of elements
type DiameterPeerWithStatus struct {
	// Pointer to the corresponding DiamterPeer
	Peer *diampeer.DiameterPeer

	// True when the Peer may admit requests, that is, when the PeerUp command has been received
	IsEngaged bool

	// True as soon as the Peer is created, and a PeerDown event must be received to signal that it can be closed
	IsUp bool

	// For reporting purposes
	LastStatusChange time.Time
	LastError        error
}

// Statuses of the Router
const (
	StatusOperational = int32(0)
	StatusClosing     = int32(1)
)

// Size of the channel for getting messages to route
// TODO: Anything other than 0 or 1 should be explained
const DIAMETER_REQUESTS_QUEUE_SIZE = 16

// Size of the channel for getting peer control messages
// TODO: Anything other than 0 or 1 should be explained
const PEER_CONTROL_QUEUE_SIZE = 16

// Timeout in seconds for http2 handlers
const HTTP_TIMEOUT_SECONDS = 10

// Represents a Diameter Message to be routed, either to a handler
// or to another Diameter Peer
type RoutableDiameterRequest struct {
	// Pointer to the actual Diameter message
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

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// Stauts of the Router
	status int32

	// Accepter of incoming connections
	listener net.Listener

	// Holds the Peers Table.
	// One entry for each configured peer or for peers now not configured but still not received
	// the PeerDown event
	diameterPeersTable map[string]DiameterPeerWithStatus

	// Time to check the peer status. Reload configuration and check if new connections
	// need to be established or closed
	peerTableTicker *time.Ticker

	// Passed to the DiameterPeers to receive back lifecycle events
	peerControlChannel chan interface{}

	// Used to retreive Diameter Requests
	diameterRequestsChan chan RoutableDiameterRequest

	// To send commands to this Router
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
		ci:                   config.GetPolicyConfigInstance(instanceName),
		diameterPeersTable:   make(map[string]DiameterPeerWithStatus),
		peerTableTicker:      time.NewTicker(60 * time.Second),
		peerControlChannel:   make(chan interface{}, PEER_CONTROL_QUEUE_SIZE),
		diameterRequestsChan: make(chan RoutableDiameterRequest, DIAMETER_REQUESTS_QUEUE_SIZE),
		routerControlChannel: make(chan interface{}),
		RouterDoneChannel:    make(chan struct{}),
	}

	// Configure client for handlers
	transportCfg := &http2.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	// Create an http client with timeout and http2 transport
	router.http2Client = http.Client{Timeout: HTTP_TIMEOUT_SECONDS * time.Second, Transport: transportCfg}

	go router.eventLoop()

	return &router
}

// Starts the closing process. It will set in StatusClosing stauts and wait for the peers to finish
// before sending the Done message to the RouterDoneChannel
func (router *Router) Close() {
	router.routerControlChannel <- RouterCloseCommand{}
}

// Actor model event loop
func (router *Router) eventLoop() {

	logger := config.GetLogger()

	// Server socket
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", router.ci.DiameterServerConf().BindPort))
	if err != nil {
		panic(err)
	}
	// Assign to instance variable
	router.listener = listener

	// Accepter loop
	go func() {
		logger.Info("diameter server accepting connections")
		for {
			connection, err := router.listener.Accept()
			if err != nil {
				// Use atomic to avoid races
				if atomic.LoadInt32(&router.status); router.status != StatusClosing {
					logger.Info("error accepting connection", err)
					panic(err)
				}
				// We are closing business. Finish acceptor loop
				return
			}

			remoteAddr, _, _ := net.SplitHostPort(connection.RemoteAddr().String())
			logger.Infof("accepted connection from %s", remoteAddr)

			remoteIPAddr, _ := net.ResolveIPAddr("", remoteAddr)
			peersConf := router.ci.PeersConf()
			if !peersConf.ValidateIncomingAddress("", remoteIPAddr.IP) {
				logger.Infof("invalid peer %s\n", remoteIPAddr)
				connection.Close()
				continue
			}

			// Create peer for the accepted connection and start it
			// The addition to the peers table will be done later,
			// after the PeerUp evventis received and checking that there is not a duplicate.
			// Declares, as handler for the Peer, a function that injects here a message to be routed!
			diampeer.NewPassiveDiameterPeer(
				router.instanceName,
				router.peerControlChannel,
				connection,
				// The handler injects me the message
				func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
					return router.RouteDiameterRequest(request, 0)
				},
			)
		}
	}()

	// First pass
	router.updatePeersTable()

routerEventLoop:
	for {
	messageHandler:
		select {

		// Handle peer lifecycle messages for this Router
		case m := <-router.routerControlChannel:
			switch m.(type) {
			case RouterCloseCommand:
				// Set the status
				atomic.StoreInt32(&router.status, StatusClosing)

				// Close the listener. The acceptor loop will exit
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

					// If we are closing the shop, set peer down
					if atomic.LoadInt32(&router.status) == StatusClosing {
						v.Sender.SetDown()
					}
				} else {
					// Peer not configured. There must have been a race condition
					logger.Warnf("unconfigured peer %s. Disengaging", v.DiameterHost)
					v.Sender.SetDown()
				}

				// Update the PeersTable in instrumentation
				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

			case diampeer.PeerDownEvent:
				// Closing may take time
				logger.Infof("closing %s", v.Sender.PeerConfig.DiameterHost)
				go v.Sender.Close()

				// Look for peer based on pointer identity, not OriginHost identity
				// Mark as disengaged. Ignore if not found (might be unconfigured
				// or taken over by another peer)
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

				// If origin-host now not in configuration, remove from peers table. It was there
				// temporarily, until the PeerDown event is received
				diameterPeersConf := router.ci.PeersConf()
				if peer, found := diameterPeersConf[v.Sender.PeerConfig.DiameterHost]; !found {
					delete(router.diameterPeersTable, peer.DiameterHost)
				}

				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

				// Check if we must exit
				if atomic.LoadInt32(&router.status) == StatusClosing {
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
			route, err := router.ci.RoutingRulesConf().FindDiameterRoute(
				rdr.Message.GetStringAVP("Destination-Realm"),
				rdr.Message.ApplicationName,
				false)

			if err != nil {
				instrumentation.PushRouterRouteNotFound("", rdr.Message)
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
				instrumentation.PushRouterNoAvailablePeer("", rdr.Message)
				rdr.RChan <- fmt.Errorf("resquest not sent: no engaged peer")
				close(rdr.RChan)

			} else if len(route.Handlers) > 0 {
				// Handle locally

				// For handlers there is not such a thing as fixed policy
				var destinationURLs []string
				destinationURLs = append(destinationURLs, route.Handlers...)
				rand.Seed(time.Now().UnixNano())
				rand.Shuffle(len(destinationURLs), func(i, j int) { destinationURLs[i], destinationURLs[j] = destinationURLs[j], destinationURLs[i] })

				// Send to the handler asynchronously
				go func(respChan chan interface{}, diameterRequest *diamcodec.DiameterMessage) {

					// Make sure the response channel is closed
					defer close(respChan)

					answer, err := httphandler.HttpDiameterRequest(router.http2Client, destinationURLs[0], diameterRequest)
					if err != nil {
						logger.Error(err.Error())
						rdr.RChan <- err
					} else {
						// Add the Origin-Host and Origin-Realm, that are not set by the handler
						// because it lacks that configuration
						answer.AddOriginAVPs(router.ci)
						rdr.RChan <- answer
					}

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
	if atomic.LoadInt32(&router.status) == StatusClosing {
		return
	}

	// Get the current configuration
	diameterPeersConf := router.ci.PeersConf()

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
			diameterPeersConf := router.ci.PeersConf()
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
