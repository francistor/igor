package config

import (
	"encoding/json"
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
	currentRadiusHandlers     RadiusHandlers

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
	if cerr = policyConfig.UpdateDiameterPeers(); cerr != nil {
		panic(cerr)
	}
	if cerr = policyConfig.UpdateDiameterRoutingRules(); cerr != nil {
		panic(cerr)
	}

	// Load radius configuration
	if cerr = policyConfig.UpdateRadiusServerConfig(); cerr != nil {
		panic(cerr)
	}
	if cerr = policyConfig.UpdateRadiusClients(); cerr != nil {
		panic(cerr)
	}
	if cerr = policyConfig.UpdateRadiusServers(); cerr != nil {
		panic(cerr)
	}
	if cerr = policyConfig.UpdateRadiusHandlers(); cerr != nil {
		panic(cerr)
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

// Retrieves the diameter server configuration, forcing a refresh
func (c *PolicyConfigurationManager) getDiameterServerConfig() (DiameterServerConfig, error) {
	dsc := DiameterServerConfig{}
	dc, err := c.CM.GetConfigObject("diameterServer.json", true)
	if err != nil {
		return dsc, err
	}
	if err := json.Unmarshal(dc.RawBytes, &dsc); err != nil {
		return dsc, err
	}
	return dsc, nil
}

// Updates the diameter server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateDiameterServerConfig() error {
	dsc, err := c.getDiameterServerConfig()
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

// Retrieves the radius server configuration, corcing a refresh
func (c *PolicyConfigurationManager) getRadiusServerConfig() (RadiusServerConfig, error) {
	rsc := RadiusServerConfig{}
	rc, err := c.CM.GetConfigObject("radiusServer.json", true)
	if err != nil {
		return rsc, err
	}
	if err := json.Unmarshal(rc.RawBytes, &rsc); err != nil {
		return rsc, err
	}
	return rsc, nil
}

// Updates the radius server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusServerConfig() error {
	rsc, err := c.getRadiusServerConfig()
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

// Retrieves the radius clients configuration, forcing a refresh
func (c *PolicyConfigurationManager) getRadiusClientsConfig() (RadiusClients, error) {

	// To store the parsed JSON
	var clientsArray []RadiusClient

	// To be returned
	radiusClients := make(RadiusClients)
	rc, err := c.CM.GetConfigObject("radiusClients.json", true)
	if err != nil {
		return radiusClients, err
	}
	if err := json.Unmarshal(rc.RawBytes, &clientsArray); err != nil {
		return radiusClients, err
	}

	// Fill the map
	for _, c := range clientsArray {
		radiusClients[c.IPAddress] = c
	}

	return radiusClients, nil
}

// Updates the radius clients configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusClients() error {
	radiusClients, err := c.getRadiusClientsConfig()
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius Clients configuration: %w", err)
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

// Retrieves the radius servers configuration, forcing a refresh
func (c *PolicyConfigurationManager) getRadiusServersConfig() (RadiusServers, error) {

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

	rc, err := c.CM.GetConfigObject("radiusServers.json", true)
	if err != nil {
		return radiusServers, err
	}
	if err := json.Unmarshal(rc.RawBytes, &radiusServersArray); err != nil {
		return radiusServers, err
	}

	// Do the formating, as maps indexed by name
	for _, rs := range radiusServersArray.Servers {
		radiusServers.Servers[rs.Name] = rs
	}
	for _, rg := range radiusServersArray.ServerGroups {
		radiusServers.ServerGroups[rg.Name] = rg
	}

	return radiusServers, nil
}

// Updates the radius servers configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusServers() error {
	radiusServers, err := c.getRadiusServersConfig()
	if err != nil {
		return fmt.Errorf("could not retrieve the Radius Servers configuration: %w", err)
	}
	c.currentRadiusServers = radiusServers
	return nil
}

// Retrieves the contents of the global variable containing the radius servers configuration
func (c *PolicyConfigurationManager) RadiusServersConf() RadiusServers {
	return c.currentRadiusServers
}

// Holds the radius handlers configuration
type RadiusHandlers struct {
	AuthHandlers []string
	AcctHandlers []string
	COAHandlers  []string
}

// Retrieves the radius handlers configuration, forcing a refresh
func (c *PolicyConfigurationManager) getRadiusHandlersConfig() (RadiusHandlers, error) {
	var radiusHandlers RadiusHandlers
	rc, err := c.CM.GetConfigObject("radiusHandlers.json", true)
	if err != nil {
		return radiusHandlers, err
	}
	if err := json.Unmarshal(rc.RawBytes, &radiusHandlers); err != nil {
		return radiusHandlers, err
	}
	return radiusHandlers, nil
}

// Updates the radius handlers configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusHandlers() error {
	radiusHandlers, error := c.getRadiusHandlersConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Radius Handlers configuration: %w", error)
	}
	c.currentRadiusHandlers = radiusHandlers
	return nil
}

// Retrieves the contents of the global variable containing the radius handlers configuration
func (c *PolicyConfigurationManager) RadiusHandlersConf() RadiusHandlers {
	return c.currentRadiusHandlers
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

// Retrieves the Routes configuration, forcing a refresh
func (c *PolicyConfigurationManager) getDiameterRoutingRules() (DiameterRoutingRules, error) {
	var routingRules []DiameterRoutingRule
	rr, err := c.CM.GetConfigObject("diameterRoutes.json", true)
	if err != nil {
		return routingRules, err
	}
	err = json.Unmarshal(rr.RawBytes, &routingRules)
	if err != nil {
		return routingRules, err
	}
	return routingRules, nil
}

// Updates the diameter routing rules configuration in the global variable
func (c *PolicyConfigurationManager) UpdateDiameterRoutingRules() error {
	drr, error := c.getDiameterRoutingRules()
	if error != nil {
		return fmt.Errorf("could not retrieve the Diameter Routing Rules configuration: %w", error)
	}
	c.currentRoutingRules = drr
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

// Retrieves the Peers configuration, forcing a refresh
func (c *PolicyConfigurationManager) getDiameterPeers() (DiameterPeers, error) {

	// Read from JSON
	var peers []DiameterPeer

	// To be returned
	peersMap := make(map[string]DiameterPeer)

	dp, err := c.CM.GetConfigObject("diameterPeers.json", true)
	if err != nil {
		return peersMap, err
	}
	err = json.Unmarshal(dp.RawBytes, &peers)
	if err != nil {
		return peersMap, err
	}

	// Cooking. Adding parsed origin network and build as a map per diameter-host
	for i := range peers {
		_, ipNet, err := net.ParseCIDR(peers[i].OriginNetwork)
		if err != nil {
			return peersMap, err
		}
		peers[i].OriginNetworkCIDR = *ipNet
		peersMap[peers[i].DiameterHost] = peers[i]
	}

	return peersMap, nil
}

// Updates the DiameterPeers configuration
func (c *PolicyConfigurationManager) UpdateDiameterPeers() error {
	dp, err := c.getDiameterPeers()
	if err != nil {
		return fmt.Errorf("could not retrieve the Peers configuration: %w", err)
	}
	c.currentDiameterPeers = dp
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

// Retrieves the http router configuration, forcing a refresh
func (c *PolicyConfigurationManager) getHttpRouterConfig() (HttpRouterConfig, error) {
	hrc := HttpRouterConfig{}
	co, err := c.CM.GetConfigObject("httpRouter.json", true)
	if err != nil {
		return hrc, err
	}
	if err := json.Unmarshal(co.RawBytes, &hrc); err != nil {
		return hrc, err
	}
	return hrc, nil
}

// Updates the diameter server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateHttpRouterConfig() error {
	hrc, err := c.getHttpRouterConfig()
	if err != nil {
		return fmt.Errorf("could not retrieve the Diameter Server configuration: %w", err)
	}
	c.currentHttpRouterConfig = hrc
	return nil
}

// Retrieves the contents of the global variable containing the diameter server configuration
func (c *PolicyConfigurationManager) HttpRouterConf() HttpRouterConfig {
	return c.currentHttpRouterConfig
}
