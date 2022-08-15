package router

import (
	"crypto/tls"
	"fmt"
	"igor/config"
	"igor/httphandler"
	"igor/instrumentation"
	"igor/radiusclient"
	"igor/radiuscodec"
	"igor/radiusserver"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

// RadiusRouter
// Starts an UDP server socket
//
// Receives RoutableRadiusPacket messages, which contain a radius packet plus the specification of the server where
// to send it or, if empty, If empty, handles it.
// Handling can be done with the registered http handler or with the specified handler function
//
// When sending packets to other radius servers, the router obtains the final radius enpoint by analyzing the radius group,
// and sends the packet to the RadiusClient. It also manages the request-level retries.
//
// The status of the radius servers is kept on a table. Radius Server are marked as "down" when the number of timeouts in a row
// exceeds the configured value

// Keeps the status of the Radius Server
// Only declared servers have status
type RadiusServerWithStatus struct {
	// Basic RadiusServer configuration object
	conf config.RadiusServer

	// True when the Server may admit requests
	// In order to make a more efficient comparison sometimes than looking at the unavailableUntil date
	isAvailable bool

	// Current errors in a row
	numErrors int

	// Quarantined time. If in the past, the server is not quarantined
	unavailableUntil time.Time
}

// Encapsulates the data passed to the RadiusClient, once the routing has been
// performed
type RadiusRouteParam struct {
	endpoint   string
	originPort int
	secret     string
	serverName string
}

// Message to signal that we should send the RadiusServer Metrics
type UpdateRadiusMetrics struct {
}

// To signal the result of each operation and update the radius server status
type RadiusRequestResult struct {
	serverName string
	ok         bool
}

type RadiusRouter struct {
	// Configuration instance
	instanceName string

	// Configuration instance object
	ci *config.PolicyConfigurationManager

	// Status of the upstream radius servers declared in the configuration
	radiusServersTable map[string]*RadiusServerWithStatus

	// Used to retreive Radius Requests
	radiusRequestsChan chan RoutableRadiusRequest

	// Control channel
	routerControlChan chan interface{}

	// HTTP2 client
	http2Client http.Client

	// Radius Client
	radiusClient *radiusclient.RadiusClient

	// RadiusServers
	authServer *radiusserver.RadiusServer
	acctServer *radiusserver.RadiusServer
	coaServer  *radiusserver.RadiusServer

	// Function to handle messages not sent to http handlers
	localHandler radiusserver.RadiusPacketHandler

	// Status
	status int32

	// To make sure we wait for the goroutines to end
	wg sync.WaitGroup
}

// Creates and runs a Router
func NewRadiusRouter(instanceName string, localHandler radiusserver.RadiusPacketHandler) *RadiusRouter {

	radiusServerConf := config.GetPolicyConfigInstance(instanceName).RadiusServerConf()

	router := RadiusRouter{
		instanceName:       instanceName,
		ci:                 config.GetPolicyConfigInstance(instanceName),
		radiusServersTable: make(map[string]*RadiusServerWithStatus),
		radiusRequestsChan: make(chan RoutableRadiusRequest, RADIUS_REQUESTS_QUEUE_SIZE),
		routerControlChan:  make(chan interface{}, CONTROL_QUEUE_SIZE),
		radiusClient:       radiusclient.NewRadiusClient(config.GetPolicyConfigInstance(instanceName)),
		localHandler:       localHandler,
	}

	// The handler function sends the request to this router, signailling that it must not be sent to
	// an upstream server (destination = "")
	handler := func(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
		return router.RouteRadiusRequest("", request, 0, 0, 0, "")
	}

	if radiusServerConf.AuthPort != 0 {
		router.authServer = radiusserver.NewRadiusServer(router.ci, radiusServerConf.BindAddress, radiusServerConf.AuthPort, handler)
	}
	if radiusServerConf.AcctPort != 0 {
		router.acctServer = radiusserver.NewRadiusServer(router.ci, radiusServerConf.BindAddress, radiusServerConf.AcctPort, handler)
	}
	if radiusServerConf.CoAPort != 0 {
		router.coaServer = radiusserver.NewRadiusServer(router.ci, radiusServerConf.BindAddress, radiusServerConf.CoAPort, handler)
	}

	// Create an http client with timeout and http2 transport
	router.http2Client = http.Client{
		Timeout: HTTP_TIMEOUT_SECONDS * time.Second,
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates

		},
	}

	router.updateRadiusServersTable()
	router.routerControlChan <- UpdateRadiusMetrics{}

	go router.eventLoop()

	return &router
}

// Starts the closing process. It will set in StatusTerminated stauts and send the down signal for the
// client socket
func (router *RadiusRouter) SetDown() {
	router.routerControlChan <- RouterSetDownCommand{}
}

// Waits until the Router is finished
func (router *RadiusRouter) Close() {

	// Servers
	if router.authServer != nil {
		router.authServer.Close()
	}
	if router.acctServer != nil {
		router.acctServer.Close()
	}
	if router.coaServer != nil {
		router.coaServer.Close()
	}

	// Client
	router.radiusClient.Close()

	// Wait until all goroutines exit, including timers in outstanding requests
	router.wg.Wait()

	// Terminate event loop
	router.routerControlChan <- RouterCloseCommand{}

	// Close channels
	close(router.radiusRequestsChan)
	close(router.routerControlChan)
}

// Actor Model
func (router *RadiusRouter) eventLoop() {
	for {
		select {

		case in := <-router.routerControlChan:
			switch v := in.(type) {

			case RouterCloseCommand:

				// Terminate the event loop
				return

			case RouterSetDownCommand:

				router.status = StatusTerminated

				// Close the client. This will cancel all requests
				router.radiusClient.SetDown()

				// Terminate the event loop
				// FRG return

			case UpdateRadiusMetrics:

				instrumentation.PushRadiusServersTable(router.instanceName, router.buildRadiusServersTable())

				// Sent after each radius request, to keep track of the status of the servers
			case RadiusRequestResult:

				// Update errors and set unavailable if necessary
				// Moving from unavailable to available will be done only by quarantine expiration
				if rsws, found := router.radiusServersTable[v.serverName]; found {
					if rsws.isAvailable {
						if !v.ok {
							// Take the error into account
							// Notice that only the tries, *not the serverTries*, increment the number of errors
							rsws.numErrors++
							if rsws.numErrors >= rsws.conf.ErrorLimit {
								rsws.isAvailable = false
								now := time.Now()
								rsws.unavailableUntil = now.Add(time.Duration(rsws.conf.QuarantineTimeSeconds) * time.Second)
								router.routerControlChan <- UpdateRadiusMetrics{}
							}
						} else {
							// Reset the counter when a success comes
							rsws.numErrors = 0
						}
					}

					// All other cases are ignored. If the server is not available, will remain in that state until the
					// quarantine is elapsed

				}
				// Server will not be found if has not got a name (not declared in configuration as part of server group)
			}

		case rrr := <-router.radiusRequestsChan:

			// If closing, do not serve more requests
			if router.status == StatusTerminated {
				rrr.RChan <- fmt.Errorf("router terminated")
				close(rrr.RChan)
				// Corresponding to RouteRadiusRequest
				router.wg.Done()
				continue
			}

			if rrr.Destination != "" {
				// Route the message to upstream server
				routeParams := router.getRouteParams(rrr)
				if len(routeParams) == 0 {
					rrr.RChan <- fmt.Errorf("no server available to send request")
					close(rrr.RChan)
				} else {
					// Retry loop
					router.wg.Add(1)
					go func(rps []RadiusRouteParam, req RoutableRadiusRequest) {
						defer router.wg.Done()
						for _, routeParam := range rps {
							rchan := make(chan interface{}, 1)
							router.radiusClient.RadiusExchange(routeParam.endpoint, routeParam.originPort, req.Packet, req.PerRequestTimeout, req.ServerTries, routeParam.secret, rchan)
							response := <-rchan
							// rchan closed in RadiusExchange
							switch v := response.(type) {
							case *radiuscodec.RadiusPacket:
								req.RChan <- response
								close(req.RChan)
								router.routerControlChan <- RadiusRequestResult{serverName: routeParam.serverName, ok: true}
								return
							case error:
								router.routerControlChan <- RadiusRequestResult{serverName: routeParam.serverName, ok: false}
								config.GetLogger().Warnf("error in answer from %s %s: %s", routeParam.serverName, routeParam.endpoint, v.Error())
							}
						}
						req.RChan <- fmt.Errorf("answer not received after %d tries", len(rps))
						close(req.RChan)

					}(routeParams, rrr)
				}

			} else {
				// Handle the message
				rh := router.ci.RadiusHandlersConf()
				var destinationURLs []string
				switch rrr.Packet.Code {
				case radiuscodec.ACCESS_REQUEST:
					destinationURLs = rh.AuthHandlers
				case radiuscodec.ACCOUNTING_REQUEST:
					destinationURLs = rh.AcctHandlers
				case radiuscodec.COA_REQUEST:
					destinationURLs = rh.COAHandlers
				default:
					rrr.RChan <- fmt.Errorf("server received a not request packet")
					close(rrr.RChan)
					// Corresponding to the one in RouteRadiusRequest
					router.wg.Done()
					continue
				}

				// Handle locally
				if len(destinationURLs) == 0 {
					// Send to local handler asyncronously
					go func(rc chan interface{}, radiusPacket *radiuscodec.RadiusPacket) {
						resp, err := router.localHandler(radiusPacket)
						if err != nil {
							rc <- err
						} else {
							rc <- resp
						}
						close(rrr.RChan)
					}(rrr.RChan, rrr.Packet)

				} else {
					// Send to http handler

					// Select one destination randomly
					rand.Shuffle(len(destinationURLs), func(i, j int) { destinationURLs[i], destinationURLs[j] = destinationURLs[j], destinationURLs[i] })

					// Send to the handler asynchronously
					go func(rchan chan interface{}, radiusPacket *radiuscodec.RadiusPacket) {

						// Make sure the response channel is closed
						defer func() {
							close(rchan)
						}()

						response, err := httphandler.HttpRadiusRequest(router.http2Client, destinationURLs[0], radiusPacket)
						if err != nil {
							config.GetLogger().Error(fmt.Sprintf("http handler error: %s", err.Error()))
							rchan <- err
						} else {
							rchan <- response
						}

					}(rrr.RChan, rrr.Packet)
				}
			}

			// Done processing of request
			// Corresponding to the one in RouteRadiusRequest
			router.wg.Done()
		}
	}
}

// Handles or routes a radius request, depending on the contents of "endpoint". If empty, the request is handled;
// if pointing to an ipaddress:port, sent to that specific, possibly undeclared upstream server; if pointing to
// a server group, it is routed according to the availability of the servers in the group
// The total timeout wil be perRequestTimeout*tries*serverTries
func (router *RadiusRouter) RouteRadiusRequest(destination string, packet *radiuscodec.RadiusPacket,
	perRequestTimeout time.Duration, tries int, serverTries int, secret string) (*radiuscodec.RadiusPacket, error) {

	rchan := make(chan interface{}, 1)
	req := RoutableRadiusRequest{
		Destination:       destination,
		Secret:            secret,
		Packet:            packet,
		RChan:             rchan,
		PerRequestTimeout: perRequestTimeout,
		Tries:             tries,
		ServerTries:       serverTries,
	}

	// Will be Done() after processing the request message
	router.wg.Add(1)

	router.radiusRequestsChan <- req

	r := <-rchan
	switch v := r.(type) {
	case error:
		return &radiuscodec.RadiusPacket{}, v
	case *radiuscodec.RadiusPacket:
		return v, nil
	}
	panic("got an answer that was not error or pointer to radius packet")
}

// Same as RouteRadiusRequests, but does not block: executes the specified handler
func (router *RadiusRouter) RouteRadiusRequestAsync(destination string, packet *radiuscodec.RadiusPacket,
	perRequestTimeout time.Duration, tries int, serverTries int, secret string, handler func(*radiuscodec.RadiusPacket, error)) {

	rchan := make(chan interface{}, 1)
	req := RoutableRadiusRequest{
		Destination:       destination,
		Secret:            secret,
		Packet:            packet,
		RChan:             rchan,
		PerRequestTimeout: perRequestTimeout,
		Tries:             tries,
		ServerTries:       serverTries,
	}

	// Will be Done() after processing the request message
	router.wg.Add(1)

	router.radiusRequestsChan <- req

	go func(rc chan interface{}) {
		r := <-rc
		switch v := r.(type) {
		case error:
			handler(nil, v)
		case *radiuscodec.RadiusPacket:
			handler(v, nil)
		default:
			panic("got an answer that was not error or pointer to radius packet")
		}

	}(rchan)
}

// Obtains the parameters to use when sending the request to the radius client, taking into
// account all the retries. If no servers available or group not found will return an empty slice
func (router *RadiusRouter) getRouteParams(req RoutableRadiusRequest) []RadiusRouteParam {

	// The slice of routing parameters to be returned
	params := make([]RadiusRouteParam, 0)

	if strings.Contains(req.Destination, ":") {
		// Specific server
		// params will have a single entry
		// tries will not be used. Only serverTries
		originPorts := router.ci.RadiusServerConf().OriginPorts
		routeParam := RadiusRouteParam{
			endpoint:   normalizeEndpoint(req.Destination),
			originPort: originPorts[rand.Intn(len(originPorts))],
			secret:     req.Secret,
		}
		params = append(params, routeParam)

	} else {
		// Server group
		if serverGroup, found := router.ci.RadiusServersConf().ServerGroups[req.Destination]; found {

			// Filter for available servers
			availableServerNames := make([]string, 0)
			for _, serverName := range serverGroup.Servers {

				server := router.radiusServersTable[serverName]

				if server.isAvailable {
					availableServerNames = append(availableServerNames, serverName)
				} else {
					// Still could be available
					if server.unavailableUntil.Before(time.Now()) {
						server.isAvailable = true
						availableServerNames = append(availableServerNames, serverName)
						router.routerControlChan <- UpdateRadiusMetrics{}
					}
				}
			}

			nServers := len(availableServerNames)
			if nServers == 0 {
				// No servers available
				return params
			}
			initialServerIndex := 0
			if serverGroup.Policy == "random" {
				initialServerIndex = rand.Intn(nServers)
			}

			for i := 0; i < req.Tries; i++ {
				serverName := availableServerNames[(initialServerIndex+i)%nServers]
				server := router.radiusServersTable[serverName]

				// Select one client port randomly
				var originPorts []int
				if len(server.conf.OriginPorts) == 0 {
					originPorts = append(originPorts, router.ci.RadiusServerConf().OriginPorts...)
				} else {
					originPorts = append(originPorts, server.conf.OriginPorts...)
				}
				clientPort := originPorts[rand.Intn(len(originPorts))]

				// Determine destination port
				var destPort int
				switch req.Packet.Code {
				case radiuscodec.ACCESS_REQUEST:
					destPort = server.conf.AuthPort
				case radiuscodec.ACCOUNTING_REQUEST:
					destPort = server.conf.AcctPort
				default:
					destPort = server.conf.COAPort
				}

				// Build route param
				sName := normalizeIPAddress(server.conf.IPAddress)
				routeParam := RadiusRouteParam{
					endpoint:   fmt.Sprintf("%s:%d", sName, destPort),
					originPort: clientPort,
					serverName: serverName,
					secret:     server.conf.Secret,
				}
				params = append(params, routeParam)
			}
		} else {
			config.GetLogger().Errorf("%s server group not found", req.Destination)
		}
	}

	return params
}

// Builds the RadiusServerTable. Any previous information such as server status
// will be lost
func (router *RadiusRouter) updateRadiusServersTable() {

	// Get the current configuration
	serversConf := router.ci.RadiusServersConf().Servers

	// The table being built
	table := make(map[string]*RadiusServerWithStatus)

	// Populate the server table
	for _, conf := range serversConf {
		serverWithStatus := RadiusServerWithStatus{
			conf:        conf,
			isAvailable: true,
		}
		table[conf.Name] = &serverWithStatus
	}

	// And update in router object
	router.radiusServersTable = table
}

// For instrumentation
func (router *RadiusRouter) buildRadiusServersTable() instrumentation.RadiusServersTable {
	radiusServersTable := make([]instrumentation.RadiusServerTableEntry, 0)

	for serverName, rsws := range router.radiusServersTable {
		entry := instrumentation.RadiusServerTableEntry{
			ServerName:       serverName,
			IsAvailable:      rsws.isAvailable,
			UnavailableUntil: rsws.unavailableUntil,
		}
		radiusServersTable = append(radiusServersTable, entry)
	}

	return radiusServersTable
}

// If endopoint contains a name instead of an IP address, turn it into an IP
// address
func normalizeEndpoint(endpoint string) string {
	addrPort := strings.Split(endpoint, ":")
	if len(addrPort) != 2 {
		panic("bad endpoint format " + endpoint)
	}

	IPPtr, err := net.ResolveIPAddr("", addrPort[0])
	if err != nil {
		panic("bad endpoint format " + endpoint)
	}
	return IPPtr.String() + ":" + addrPort[1]
}

// If ip address contains a name, get the IP address
func normalizeIPAddress(ipAddress string) string {
	IPPtr, err := net.ResolveIPAddr("", ipAddress)
	if err != nil {
		panic("bad serer name or ip address format " + ipAddress)
	}
	return IPPtr.String()
}
