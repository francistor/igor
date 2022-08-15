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
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

// The Diameter Peer table is a map of this kind of elements
type DiameterPeerWithStatus struct {
	// Pointer to the corresponding DiamterPeer
	peer *diampeer.DiameterPeer

	// True when the Peer may admit requests, that is, when the PeerUp command has been received
	isEngaged bool

	// For reporting purposes
	lastStatusChange time.Time
	lastError        error
}

// The Router handles the lifecycle of peers and routes Diameter requests
// to the appropriate destinations
// It follows the Actor model. All actions take place in the event loop
type DiameterRouter struct {

	// Configuration instance
	instanceName string

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// Stauts of the Router. May be StatusOperational or StatusClosed
	status int32

	// Accepter of incoming connections
	listener net.Listener

	// Holds the Peers Table.
	// One entry for each configured peer or for peers now not configured but still not received
	// the PeerDown event // TODOFRG-> Check this comment
	diameterPeersTable map[string]DiameterPeerWithStatus

	// Timer to check the peer status. Reload configuration and check if new connections
	// need to be established or closed
	peerTableTicker *time.Ticker

	// Passed to the DiameterPeers to receive back lifecycle events
	peerControlChannel chan interface{}

	// Used to retreive Diameter Requests
	diameterRequestsChan chan RoutableDiameterRequest

	// To receive commands to this Router
	routerControlChannel chan interface{}

	// To signalthat the Router has shut down
	// used only to wait on Close() until finalization
	routerDoneChannel chan struct{}

	// To make sure there are no outstanding requests pending
	wg sync.WaitGroup

	// HTTP2 client for sending requests to http handlers
	http2Client http.Client

	// Local handler
	localHandler diampeer.MessageHandler
}

// Creates and runs a Router
func NewDiameterRouter(instanceName string, handler diampeer.MessageHandler) *DiameterRouter {

	router := DiameterRouter{
		instanceName:         instanceName,
		ci:                   config.GetPolicyConfigInstance(instanceName),
		diameterPeersTable:   make(map[string]DiameterPeerWithStatus),
		peerTableTicker:      time.NewTicker(PEER_CHECK_INTERVAL_SECONDS * time.Second),
		peerControlChannel:   make(chan interface{}, CONTROL_QUEUE_SIZE),
		diameterRequestsChan: make(chan RoutableDiameterRequest, DIAMETER_REQUESTS_QUEUE_SIZE),
		routerControlChannel: make(chan interface{}, CONTROL_QUEUE_SIZE),
		routerDoneChannel:    make(chan struct{}, 1),
		localHandler:         handler,
	}

	// Create an http client with timeout and http2 transport
	router.http2Client = http.Client{
		Timeout: HTTP_TIMEOUT_SECONDS * time.Second,
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
		},
	}

	go router.eventLoop()

	return &router
}

// Starts the closing process. It will set in StatusClosing stauts and wait for the peers to finish
// before sending the Done message to the RouterDoneChannel
func (router *DiameterRouter) SetDown() {
	router.routerControlChannel <- RouterSetDownCommand{}
}

// Waits until the Router is finished
func (router *DiameterRouter) Close() {
	<-router.routerDoneChannel

	router.wg.Wait()

	// Terminate the event loop
	router.routerControlChannel <- RouterCloseCommand{}
}

// Actor model event loop
func (router *DiameterRouter) eventLoop() {

	logger := config.GetLogger()

	// Server socket
	serverConf := router.ci.DiameterServerConf()
	listener, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", serverConf.BindAddress, serverConf.BindPort))
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
				// Use atomic to avoid races, because this is in reality of the eventLoop (goroutine)
				if atomic.LoadInt32(&router.status) != StatusTerminated {
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

			// Check that the incoming IP address is on the list of originCIDR for declared Peers
			if !peersConf.ValidateIncomingAddress("", remoteIPAddr.IP) {
				logger.Infof("received incoming connection from invalid peer %s\n", remoteIPAddr)
				connection.Close()
				continue
			}

			// Create peer for the accepted connection and start it
			// The addition to the peers table will be done later,
			// after the PeerUp event is received and checking that there is not a duplicate.
			// Declares, as handler for the Peer, a function that injects here a message to be routed
			diampeer.NewPassiveDiameterPeer(
				router.instanceName,
				router.peerControlChannel,
				connection,
				// The handler injects me the message
				func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
					return router.RouteDiameterRequest(request, DEFAULT_REQUEST_TIMEOUT_SECONDS*time.Second)
				},
			)
		}
	}()

	// First pass
	router.updatePeersTable()

	// FRG routerEventLoop:
	for {
	messageHandler:
		select {

		// Handle peer lifecycle messages for this Router
		case m := <-router.routerControlChannel:
			switch m.(type) {

			case RouterCloseCommand:
				// Terminate the event loop
				return

			case RouterSetDownCommand:
				// Set the status
				atomic.StoreInt32(&router.status, StatusTerminated)

				// Stop the ticker
				router.peerTableTicker.Stop()

				// Close the listener. The acceptor loop will exit
				router.listener.Close()

				// Close all peers that are up
				// TODO: Check that it is no harm to send two SetDown()
				for peer := range router.diameterPeersTable {
					if router.diameterPeersTable[peer].peer != nil {
						router.diameterPeersTable[peer].peer.SetDown()
					}
				}

				// Check if we can exit
				for peer := range router.diameterPeersTable {
					if router.diameterPeersTable[peer].peer != nil {
						break messageHandler
					}
				}

				// If here, all peers are not up
				// Signal to the outside
				close(router.routerDoneChannel)

				// FRG break routerEventLoop
			}

		case <-router.peerTableTicker.C:
			if atomic.LoadInt32(&router.status) == StatusOperational {
				// Update peers
				router.updatePeersTable()
			}

		// Receive lifecycle messages from managed Peers
		case m := <-router.peerControlChannel:
			switch v := m.(type) {
			case diampeer.PeerUpEvent:
				if peerEntry, found := router.diameterPeersTable[v.DiameterHost]; found {
					// Entry found for this peer (normal)
					if peerEntry.peer != v.Sender {
						// And is not the one reporting up. Possibly a passive Peer
						if peerEntry.peer != nil && peerEntry.isEngaged {
							// The existing peer wins. Disengage the newly received one
							v.Sender.SetDown()
							logger.Infof("keeping already engaged peer entry for %s", v.DiameterHost)
						} else {
							// The new peer wins. Disengage the existing one if there is one
							if peerEntry.peer != nil {
								peerEntry.peer.SetDown()
								logger.Infof("closing not engaged peer entry for %s", v.DiameterHost)
							}
							// Update the peers table
							router.diameterPeersTable[v.DiameterHost] = DiameterPeerWithStatus{peer: v.Sender, isEngaged: true, lastStatusChange: time.Now(), lastError: nil}
							logger.Infof("new peer entry for %s", v.DiameterHost)
						}
					} else {
						// It is the one reporting up. Only change state
						peerEntry.isEngaged = true
						peerEntry.lastStatusChange = time.Now()
						peerEntry.lastError = nil
						router.diameterPeersTable[v.DiameterHost] = peerEntry
						logger.Infof("updating peer entry for %s", v.DiameterHost)
					}

					// If we are closing the shop, set peer down
					if atomic.LoadInt32(&router.status) == StatusTerminated {
						v.Sender.SetDown()
					}
				} else {
					// Peer not configured. There must have been a race condition
					logger.Warnf("unconfigured peer %s sent PeerUp event. Disengaging", v.DiameterHost)
					v.Sender.SetDown()
				}

				// Update the PeersTable in instrumentation
				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

			case diampeer.PeerDownEvent:
				// Closing may take time. Do it in the background
				logger.Infof("closing %s", v.Sender.GetPeerConfig().DiameterHost)
				go v.Sender.Close()

				// Look for peer based on pointer identity, not OriginHost identity
				// Mark as disengaged. Ignore if not found (might be unconfigured
				// or taken over by another peer)
				for originHost, existingPeer := range router.diameterPeersTable {
					if existingPeer.peer == v.Sender {
						existingPeer.isEngaged = false
						existingPeer.lastStatusChange = time.Now()
						existingPeer.lastError = v.Error
						existingPeer.peer = nil
						router.diameterPeersTable[originHost] = existingPeer
					}
				}

				// If origin-host now not in configuration, remove from peers table. It was there
				// temporarily, until the PeerDown event is received
				diameterPeersConf := router.ci.PeersConf()
				if peer, found := diameterPeersConf[v.Sender.GetPeerConfig().DiameterHost]; !found {
					delete(router.diameterPeersTable, peer.DiameterHost)
				}

				instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())

				// Check if we must exit
				if atomic.LoadInt32(&router.status) == StatusTerminated {
					// Check if we can exit
					for peer := range router.diameterPeersTable {
						if router.diameterPeersTable[peer].peer != nil {
							break messageHandler
						}
					}

					// If here, all peers are not up
					// Signal to the outside
					close(router.routerDoneChannel)
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
				close(rdr.RChan)
			} else {
				// Route found
				logger.Debugf("Found matching rule %v", route)
				if len(route.Peers) > 0 {
					// Route to destination peer
					// If policy is "random", shuffle the destination-hosts
					var peers []string
					peers = append(peers, route.Peers...)

					if route.Policy == "random" {
						rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
					}

					for _, destinationHost := range peers {
						targetPeer := router.diameterPeersTable[destinationHost]
						if targetPeer.isEngaged {
							// Route found. Send request asyncronously
							logger.Debugf("Selected Peer: %s", destinationHost)
							go func() {
								targetPeer.peer.DiameterExchange(rdr.Message, rdr.Timeout, rdr.RChan)
							}()

							// Corresponding to RouteDiameterMessage
							router.wg.Done()
							break messageHandler
						}
					}

					// If here, could not find a peer
					instrumentation.PushRouterNoAvailablePeer("", rdr.Message)
					rdr.RChan <- fmt.Errorf("resquest not sent: no engaged peer")
					close(rdr.RChan)

				} else if len(route.Handlers) > 0 {
					// Use http handlers

					// For handlers there is not such a thing as fixed policy
					var destinationURLs []string
					destinationURLs = append(destinationURLs, route.Handlers...)
					//rand.Seed(time.Now().UnixNano())
					rand.Shuffle(len(destinationURLs), func(i, j int) { destinationURLs[i], destinationURLs[j] = destinationURLs[j], destinationURLs[i] })

					// Send to the handler asynchronously
					logger.Debugf("Selected Handler: %s", destinationURLs[0])
					go func(rchan chan interface{}, diameterRequest *diamcodec.DiameterMessage, url string) {

						// Make sure the response channel is closed
						defer close(rchan)

						answer, err := httphandler.HttpDiameterRequest(router.http2Client, url, diameterRequest)
						if err != nil {
							logger.Errorf("http handler %s returned error: %s", url, err.Error())
							rchan <- err
						} else {
							// Add the Origin-Host and Origin-Realm, that are not set by the handler
							// because it lacks that configuration
							answer.AddOriginAVPs(router.ci)
							rchan <- answer
						}

					}(rdr.RChan, rdr.Message, destinationURLs[0])

				} else {
					// Handle locally
					go func(rchan chan interface{}, diameterRequest *diamcodec.DiameterMessage) {

						// Make sure the response channel is closed
						defer func() {
							close(rchan)
						}()

						answer, err := router.localHandler(diameterRequest)
						if err != nil {
							logger.Errorf("local handler returned error: %s", err.Error())
							rchan <- err
						} else {
							// Add the Origin-Host and Origin-Realm, that are not set by the handler
							// because it lacks that configuration
							answer.AddOriginAVPs(router.ci)
							rchan <- answer
						}

					}(rdr.RChan, rdr.Message)
				}
			}

			// Corresponding to RouteDiameterRequest
			router.wg.Done()
		}
	}
}

// Sends a DiameterMessage and returns the answer. Blocking
func (router *DiameterRouter) RouteDiameterRequest(request *diamcodec.DiameterMessage, timeout time.Duration) (*diamcodec.DiameterMessage, error) {
	responseChannel := make(chan interface{}, 1)

	routableRequest := RoutableDiameterRequest{
		Message: request,
		RChan:   responseChannel,
		Timeout: timeout,
	}

	// Will be Done() after processing the message
	router.wg.Add(1)

	router.diameterRequestsChan <- routableRequest

	r := <-responseChannel
	switch v := r.(type) {
	case error:
		return &diamcodec.DiameterMessage{}, v
	case *diamcodec.DiameterMessage:
		return v, nil
	}
	panic("got an answer that was not error or pointer to diameter message")
}

// Same as RouteDiameterRequest but non blocking: executes the handler asyncrhronously
func (router *DiameterRouter) RouteDiameterRequestAsync(request *diamcodec.DiameterMessage, timeout time.Duration, handler func(*diamcodec.DiameterMessage, error)) {
	rchan := make(chan interface{}, 1)

	routableRequest := RoutableDiameterRequest{
		Message: request,
		RChan:   rchan,
		Timeout: timeout,
	}

	// Will be Done() after processing the message
	router.wg.Add(1)

	router.diameterRequestsChan <- routableRequest

	go func(rc chan interface{}) {
		r := <-rc
		switch v := r.(type) {
		case error:
			handler(nil, v)
		case *diamcodec.DiameterMessage:
			handler(v, nil)
		default:
			panic("got an answer that was not error or pointer to diameter message")
		}

	}(rchan)
}

// Takes the current map of DiameterPeers and generates a new one based on the current configuration
// There is an entry per configured peer, either active or passive. Entries unconfigured for which the
// PeerDown command has not yet been received will still be present in the table, to be removed later.
// Active peers will have a peer pointer that is removed when receiving the down event, and another peer
// is created when checking the peers table.
// Passive peers are initially created with a nil peer pointer
func (router *DiameterRouter) updatePeersTable() {

	// Get the current configuration
	diameterPeersConf := router.ci.PeersConf()

	// Force non configured peers to disengage
	// The real removal from the table will take place when the PeerDownEvent is received
	for existingDH, p := range router.diameterPeersTable {
		if _, found := diameterPeersConf[existingDH]; !found {
			// The table will be updated and this peer removed wheh the PeerDown event is received
			p.peer.SetDown()
			p.isEngaged = false
		}
	}

	// Make sure an entry exists for each configured peer, and create a Peer if active and has no backing peer
	for _, peerConfig := range diameterPeersConf {
		p, found := router.diameterPeersTable[peerConfig.DiameterHost]
		if peerConfig.ConnectionPolicy == "active" {
			// Create a new one of not existing or there was no backing peer (possibly because got down)
			if !found || p.peer == nil {
				diamPeer := diampeer.NewActiveDiameterPeer(router.instanceName, router.peerControlChannel, peerConfig, func(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
					return router.RouteDiameterRequest(request, DEFAULT_REQUEST_TIMEOUT_SECONDS*time.Second)
				})
				router.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{peer: diamPeer, isEngaged: false, lastStatusChange: time.Now()}
			}
		} else {
			if !found {
				router.diameterPeersTable[peerConfig.DiameterHost] = DiameterPeerWithStatus{peer: nil, isEngaged: false, lastStatusChange: time.Now()}
			}
		}
	}

	instrumentation.PushDiameterPeersStatus(router.instanceName, router.buildPeersStatusTable())
}

// Generates the DiameterPeersTableEntry for instrumetation purposes, using the current
// internal table and shuffling the fields as necessary to adjust the contents
func (router *DiameterRouter) buildPeersStatusTable() instrumentation.DiameterPeersTable {
	peerTable := make([]instrumentation.DiameterPeersTableEntry, 0)

	for diameterHost, peerStatus := range router.diameterPeersTable {
		var ipAddress string = ""
		var connectionPolicy = ""
		if peerStatus.peer != nil {
			// Take from effective values
			peerConfig := peerStatus.peer.GetPeerConfig()
			ipAddress = peerConfig.IPAddress
			connectionPolicy = peerConfig.ConnectionPolicy
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
			IsEngaged:        peerStatus.isEngaged,
			LastStatusChange: peerStatus.lastStatusChange,
			LastError:        peerStatus.lastError,
		}
		peerTable = append(peerTable, instrumentationEntry)
	}

	return peerTable
}
