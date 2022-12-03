package config

import (
	"fmt"
	"net"
)

// Manages the configuration items for policy
type PolicyConfigurationManager struct {
	CM ConfigurationManager

	currentDiameterServerConfig DiameterServerConfig
	currentRoutingRules         DiameterRoutingRules
	currentDiameterPeers        DiameterPeers

	currentRadiusServerConfig RadiusServerConfig
	currentRadiusClients      RadiusClients
	currentRadiusServers      RadiusServers
	currentRadiusHttpHandlers RadiusHttpHandlers

	currentHttpRouterConfig HttpRouterConfig
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetPolicyConfig().
// To retrieve a specific instance, use GetPolicyConfigInstance(<instance-name>)
var policyConfigs []*PolicyConfigurationManager = make([]*PolicyConfigurationManager, 0)

// Adds a Policy (Radius and Diameter) configuration object with the specified name to the list of policyConfigs
// if isDefault is true, also initializes the logger and the dictionaries, which are shared among all instances
func InitPolicyConfigInstance(bootstrapFile string, instanceName string, isDefault bool) *PolicyConfigurationManager {

	// Check not already instantiated
	for i := range policyConfigs {
		if policyConfigs[i].CM.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	policyConfig := PolicyConfigurationManager{CM: NewConfigurationManager(bootstrapFile, instanceName)}
	policyConfigs = append(policyConfigs, &policyConfig)

	// Initialize logger and dictionary, if default
	if isDefault {
		initLogger(&policyConfig.CM)
		initDictionaries(&policyConfig.CM)
	}

	// Load diameter configuraton
	var cerr error
	if cerr = policyConfig.UpdateDiameterServerConfig(); cerr != nil {
		panic(cerr)
	}
	if policyConfig.currentDiameterServerConfig.BindAddress != "" {
		if cerr = policyConfig.UpdateDiameterPeers(); cerr != nil {
			panic(cerr)
		}
		if cerr = policyConfig.UpdateDiameterRoutingRules(); cerr != nil {
			panic(cerr)
		}
	} else {
		fmt.Println("diameter server not configured")
	}

	// Load radius configuration
	if cerr = policyConfig.UpdateRadiusServerConfig(); cerr != nil {
		panic(cerr)
	}
	if policyConfig.currentRadiusServerConfig.BindAddress != "" {
		if cerr = policyConfig.UpdateRadiusClients(); cerr != nil {
			panic(cerr)
		}
		if cerr = policyConfig.UpdateRadiusServers(); cerr != nil {
			panic(cerr)
		}
		if cerr = policyConfig.UpdateRadiusHttpHandlers(); cerr != nil {
			panic(cerr)
		}
	} else {
		fmt.Println("radius server not configured")
	}

	// Load http router configuration
	if cerr = policyConfig.UpdateHttpRouterConfig(); cerr != nil {
		panic(cerr)
	}

	return &policyConfig
}

// Retrieves a specific configuration instance
func GetPolicyConfigInstance(instanceName string) *PolicyConfigurationManager {

	for i := range policyConfigs {
		if policyConfigs[i].CM.instanceName == instanceName {
			return policyConfigs[i]
		}
	}

	panic("configuraton instance <" + instanceName + "> not configured")
}

// Retrieves the default configuration instance, which is the first one in the list
func GetPolicyConfig() *PolicyConfigurationManager {
	return policyConfigs[0]
}

///////////////////////////////////////////////////////////////////////////////

type DiameterServerConfig struct {
	BindAddress          string
	BindPort             int
	DiameterHost         string
	DiameterRealm        string
	VendorId             int
	ProductName          string
	FirmwareRevision     int
	PeerCheckTimeSeconds int
}

// Updates the diameter server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateDiameterServerConfig() error {
	dsc := DiameterServerConfig{}
	err := c.CM.BuildJSONConfigObject("diameterServer.json", &dsc)
	if err != nil {
		return fmt.Errorf("could not retrieve the Diameter Server configuration: %w", err)
	}
	c.currentDiameterServerConfig = dsc
	return nil
}

// Retrieves the contents of the global variable containing the diameter server configuration
func (c *PolicyConfigurationManager) DiameterServerConf() DiameterServerConfig {
	return c.currentDiameterServerConfig
}

// /////////////////////////////////////////////////////////////////////////////
type RadiusServerConfig struct {
	BindAddress               string
	AuthPort                  int
	AcctPort                  int
	CoAPort                   int
	OriginPorts               []int
	HttpHandlerTimeoutSeconds int
}

// Updates the radius server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusServerConfig() error {
	rsc := RadiusServerConfig{}
	err := c.CM.BuildJSONConfigObject("radiusServer.json", &rsc)
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius Server configuration: %w", err)
	}
	c.currentRadiusServerConfig = rsc
	return nil
}

// Retrieves the contents of the global variable containing the radius server configuration
func (c *PolicyConfigurationManager) RadiusServerConf() RadiusServerConfig {
	return c.currentRadiusServerConfig
}

// Holds the configuration of a Radius Client
type RadiusClient struct {
	Name             string
	IPAddress        string
	Secret           string
	ClientClass      string
	ClientProperties map[string]string
}

// Holds the configuration of all Radius Clients, indexed by IP address
type RadiusClients map[string]RadiusClient

// Updates the radius clients configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusClients() error {

	// To store the parsed JSON
	var clientsArray []RadiusClient

	// To be returned
	radiusClients := make(RadiusClients)
	err := c.CM.BuildJSONConfigObject("radiusClients.json", &clientsArray)
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius clients configuration: %w", err)
	}

	// Fill the map
	for _, c := range clientsArray {
		radiusClients[c.IPAddress] = c
	}

	c.currentRadiusClients = radiusClients

	return nil
}

// Retrieves the contents of the global variable containing the radius clients configuration
func (c *PolicyConfigurationManager) RadiusClientsConf() RadiusClients {
	return c.currentRadiusClients
}

// Holds the configuration for an upstream Radius Server
type RadiusServer struct {
	Name                  string
	IPAddress             string
	Secret                string
	AuthPort              int
	AcctPort              int
	COAPort               int
	OriginPorts           []int
	ErrorLimit            int
	QuarantineTimeSeconds int
}

// Holds the configuration of a Radius Server Group
type RadiusServerGroup struct {
	Name    string
	Servers []string

	// policy may be "fixed" or "random"
	Policy string
}

// Holds the RadiusServers and Groups configuration, as stored in the radiusServers.json file
type RadiusServers struct {
	Servers      map[string]RadiusServer
	ServerGroups map[string]RadiusServerGroup
}

// Updates the radius servers configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusServers() error {

	// To unmarshal from JSON
	var radiusServersArray struct {
		Servers      []RadiusServer
		ServerGroups []RadiusServerGroup
	}

	// Returned value, with maps indexed by name
	radiusServers := RadiusServers{
		Servers:      make(map[string]RadiusServer),
		ServerGroups: make(map[string]RadiusServerGroup),
	}

	err := c.CM.BuildJSONConfigObject("radiusServers.json", &radiusServersArray)
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius Servers configuration: %w", err)
	}

	// Do the formating, as maps indexed by name
	for _, rs := range radiusServersArray.Servers {
		radiusServers.Servers[rs.Name] = rs
	}
	for _, rg := range radiusServersArray.ServerGroups {
		radiusServers.ServerGroups[rg.Name] = rg
	}

	c.currentRadiusServers = radiusServers
	return nil
}

// Retrieves the contents of the global variable containing the radius servers configuration
func (c *PolicyConfigurationManager) RadiusServersConf() RadiusServers {
	return c.currentRadiusServers
}

// Holds the radius handlers configuration
type RadiusHttpHandlers struct {
	AuthHandlers []string
	AcctHandlers []string
	COAHandlers  []string
}

// Updates the radius handlers configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusHttpHandlers() error {
	var radiusHttpHandlers RadiusHttpHandlers
	err := c.CM.BuildJSONConfigObject("radiusHttpHandlers.json", &radiusHttpHandlers)
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius HttpHandlers configuration: %w", err)
	}
	c.currentRadiusHttpHandlers = radiusHttpHandlers
	return nil
}

// Retrieves the contents of the global variable containing the radius handlers configuration
func (c *PolicyConfigurationManager) RadiusHttpHandlersConf() RadiusHttpHandlers {
	return c.currentRadiusHttpHandlers
}

///////////////////////////////////////////////////////////////////////////////

// Holds a Diameter Routing rule
type DiameterRoutingRule struct {
	Realm         string
	ApplicationId string
	Handlers      []string // URL to send the request to
	Peers         []string // Peers to send the request to (handler should be empty)
	Policy        string   // May be "fixed" or "random"
}

// Holds all the Diameter Routing rules
type DiameterRoutingRules []DiameterRoutingRule

// Finds the appropriate route, taking into account wildcards.
// If remote is true, force that the route is not local (has no nandler, it is sent to other peer)
func (rr DiameterRoutingRules) FindDiameterRoute(realm string, application string, remote bool) (DiameterRoutingRule, error) {
	for _, rule := range rr {
		if rule.Realm == "*" || rule.Realm == realm {
			if rule.ApplicationId == "*" || rule.ApplicationId == application {
				if !remote || (remote && len(rule.Handlers) == 0) {
					return rule, nil
				}
			}
		}
	}

	return DiameterRoutingRule{}, fmt.Errorf("rule not found for realm %s and application %s, remote: %t", realm, application, remote)
}

// Updates the diameter routing rules configuration in the global variable
func (c *PolicyConfigurationManager) UpdateDiameterRoutingRules() error {
	var routingRules []DiameterRoutingRule
	err := c.CM.BuildJSONConfigObject("diameterRoutes.json", &routingRules)
	if err != nil {
		return fmt.Errorf("could not retrieve the Diameter Routing rules configuration: %w", err)
	}
	c.currentRoutingRules = routingRules
	return nil
}

// Retrieves the contents of the global variable containing the diameter routing rules configuration
func (c *PolicyConfigurationManager) RoutingRulesConf() DiameterRoutingRules {
	return c.currentRoutingRules
}

///////////////////////////////////////////////////////////////////////////////

// Holds the configuration of a Diameter Peer
type DiameterPeer struct {
	DiameterHost            string
	IPAddress               string
	Port                    int
	ConnectionPolicy        string // May be "active" or "passive"
	OriginNetwork           string // CIDR
	OriginNetworkCIDR       net.IPNet
	WatchdogIntervalMillis  int
	ConnectionTimeoutMillis int
}

// Holds the configuration of all Diameter peers
type DiameterPeers map[string]DiameterPeer

// Check that the ip address is in the valid range for the specified diameter-host
func (dps DiameterPeers) ValidateIncomingAddress(diameterHost string, address net.IP) bool {
	if diameterHost == "" {
		// Check that at least there is a Diameter Host that allows that IP address
		for _, peer := range dps {
			if peer.OriginNetworkCIDR.Contains(address) {
				return true
			}
		}
		return false
	} else if peer, found := dps[diameterHost]; found {
		// Check that there is a match for this specified Diameter Host
		return peer.OriginNetworkCIDR.Contains(address)
	} else {
		return false
	}
}

// Updates the DiameterPeers configuration
func (c *PolicyConfigurationManager) UpdateDiameterPeers() error {
	// Read from JSON
	var peers []DiameterPeer

	// To be returned
	peersMap := make(map[string]DiameterPeer)

	err := c.CM.BuildJSONConfigObject("diameterPeers.json", &peers)
	if err != nil {
		return fmt.Errorf("could not retrieve the Diameter Peers configuration: %w", err)
	}

	// Cooking. Adding parsed origin network and build as a map per diameter-host
	for i := range peers {
		_, ipNet, err := net.ParseCIDR(peers[i].OriginNetwork)
		if err != nil {
			return fmt.Errorf("could not retrieve the Diameter Peers configuration. Bad origin address %s", peers[i].OriginNetwork)
		}
		peers[i].OriginNetworkCIDR = *ipNet
		peersMap[peers[i].DiameterHost] = peers[i]
	}

	c.currentDiameterPeers = peersMap
	return nil
}

// Returs the current DiameterPeers configuration
func (c *PolicyConfigurationManager) PeersConf() DiameterPeers {
	return c.currentDiameterPeers
}

///////////////////////////////////////////////////////////////////////////////

// Holds the configuration fot the HTTP Router
type HttpRouterConfig struct {
	BindAddress string
	BindPort    int
}

// Updates the diameter server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateHttpRouterConfig() error {
	hrc := HttpRouterConfig{}
	err := c.CM.BuildJSONConfigObject("httpRouter.json", &hrc)
	if err != nil {
		return fmt.Errorf("could not retrieve the Http Router configuration: %w", err)
	}
	c.currentHttpRouterConfig = hrc
	return nil
}

// Retrieves the contents of the global variable containing the diameter server configuration
func (c *PolicyConfigurationManager) HttpRouterConf() HttpRouterConfig {
	return c.currentHttpRouterConfig
}
