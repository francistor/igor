package config

import (
	"encoding/json"
	"fmt"
	"net"
)

type PolicyConfigurationManager struct {
	CM ConfigurationManager

	currentDiameterServerConfig DiameterServerConfig
	currentRoutingRules         DiameterRoutingRules
	currentDiameterPeers        DiameterPeers

	currentRadiusServerConfig RadiusServerConfig
	currentRadiusClients      RadiusClients
	currentRadiusServers      RadiusServers
	currentRadiusHandlers     RadiusHandlers
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig()
var policyConfigs []*PolicyConfigurationManager = make([]*PolicyConfigurationManager, 0)

// Adds a Policy (Radius and Diameter) configuration object with the specified name
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

// Retrieves the default configuration instance
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
	HttpBindAddress      string
	HttpBindPort         int
}

// Retrieves the diameter server configuration
func (c *PolicyConfigurationManager) getDiameterServerConfig() (DiameterServerConfig, error) {
	dsc := DiameterServerConfig{}
	dc, err := c.CM.GetConfigObject("diameterServer.json", true)
	if err != nil {
		return dsc, err
	}
	if err := json.Unmarshal(dc.RawBytes, &dsc); err != nil {
		fmt.Println(err)
		return dsc, err
	}
	return dsc, nil
}

func (c *PolicyConfigurationManager) UpdateDiameterServerConfig() error {
	dsc, error := c.getDiameterServerConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Diameter Server configuration: %w", error)
	}
	c.currentDiameterServerConfig = dsc
	return nil
}

func (c *PolicyConfigurationManager) DiameterServerConf() DiameterServerConfig {
	return c.currentDiameterServerConfig
}

///////////////////////////////////////////////////////////////////////////////
type RadiusServerConfig struct {
	BindAddress             string
	AuthPort                int
	AcctPort                int
	CoAPort                 int
	ClientAnonymousBasePort int
	NumAnonymousClientPorts int
}

// Retrieves the radius server configuration
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

func (c *PolicyConfigurationManager) UpdateRadiusServerConfig() error {
	rsc, error := c.getRadiusServerConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Radius Server configuration: %w", error)
	}
	c.currentRadiusServerConfig = rsc
	return nil
}

func (c *PolicyConfigurationManager) RadiusServerConf() RadiusServerConfig {
	return c.currentRadiusServerConfig
}

type RadiusClient struct {
	Name      string
	IPAddress string
	Secret    string
}

// type RadiusClients []RadiusClient
type RadiusClients map[string]RadiusClient

// Retrieves the radius clients configuration
func (c *PolicyConfigurationManager) getRadiusClientsConfig() (RadiusClients, error) {

	var clientsArray []RadiusClient

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

func (c *PolicyConfigurationManager) UpdateRadiusClients() error {
	radiusClients, error := c.getRadiusClientsConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Radius Clients configuration: %w", error)
	}
	c.currentRadiusClients = radiusClients
	return nil
}

func (c *PolicyConfigurationManager) RadiusClientsConf() RadiusClients {
	return c.currentRadiusClients
}

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

type RadiusServerGroup struct {
	Name    string
	Servers []string

	// policy may be "fixed", "random", "fixed-withclear", "random-withclear"
	Policy string
}

type RadiusServers struct {
	Servers      map[string]RadiusServer
	ServerGroups map[string]RadiusServerGroup
}

// Retrieves the radius servers configuration
func (c *PolicyConfigurationManager) getRadiusServersConfig() (RadiusServers, error) {

	// To unmarshal from JSON
	var radiusServersArray struct {
		Servers      []RadiusServer
		ServerGroups []RadiusServerGroup
	}

	// Returned value
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

	// Format
	for _, rs := range radiusServersArray.Servers {
		radiusServers.Servers[rs.Name] = rs
	}
	for _, rg := range radiusServersArray.ServerGroups {
		radiusServers.ServerGroups[rg.Name] = rg
	}

	return radiusServers, nil
}

func (c *PolicyConfigurationManager) UpdateRadiusServers() error {
	radiusServers, error := c.getRadiusServersConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Radius Servers configuration: %w", error)
	}
	c.currentRadiusServers = radiusServers
	return nil
}

func (c *PolicyConfigurationManager) RadiusServersConf() RadiusServers {
	return c.currentRadiusServers
}

type RadiusHandlers struct {
	AuthHandlers []string
	AcctHandlers []string
	COAHandlers  []string
}

// Retrieves the radius routes configuration
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

func (c *PolicyConfigurationManager) UpdateRadiusHandlers() error {
	radiusHandlers, error := c.getRadiusHandlersConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Radius Handlers configuration: %w", error)
	}
	c.currentRadiusHandlers = radiusHandlers
	return nil
}

func (c *PolicyConfigurationManager) RadiusHandlersConf() RadiusHandlers {
	return c.currentRadiusHandlers
}

///////////////////////////////////////////////////////////////////////////////

type DiameterRoutingRule struct {
	Realm         string
	ApplicationId string
	Handlers      []string // URL to send the request to
	Peers         []string // Peers to send the request to (handler should be empty)
	Policy        string   // May be "fixed" or "random"
}

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

	return DiameterRoutingRule{}, fmt.Errorf("rule not found for realm %s, application %s, remote: %t", realm, application, remote)
}

// Retrieves the Routes configuration
func (c *PolicyConfigurationManager) getDiameterRoutingRules() (DiameterRoutingRules, error) {
	var routingRules []DiameterRoutingRule
	rr, err := c.CM.GetConfigObject("diameterRoutes.json", true)
	if err != nil {
		return routingRules, err
	}
	json.Unmarshal(rr.RawBytes, &routingRules)
	return routingRules, nil
}

func (c *PolicyConfigurationManager) UpdateDiameterRoutingRules() error {
	drr, error := c.getDiameterRoutingRules()
	if error != nil {
		return fmt.Errorf("could not retrieve the Diameter Rules configuration: %w", error)
	}
	c.currentRoutingRules = drr
	return nil
}

func (c *PolicyConfigurationManager) RoutingRulesConf() DiameterRoutingRules {
	return c.currentRoutingRules
}

///////////////////////////////////////////////////////////////////////////////

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

type DiameterPeers map[string]DiameterPeer

// Gets the first Diameter Peer with the specified Diameter-Host
func (dps *DiameterPeers) FindPeer(diameterHost string) (DiameterPeer, error) {

	for _, peer := range *dps {
		if peer.DiameterHost == diameterHost {
			return peer, nil
		}
	}

	return DiameterPeer{}, fmt.Errorf("no Peer found for Origin-host %s", diameterHost)
}

func (dps *DiameterPeers) ValidateIncomingAddress(host string, address net.IP) bool {
	for _, peer := range *dps {
		if peer.OriginNetworkCIDR.Contains(address) {
			if host == "" || peer.DiameterHost == host {
				return true
			}
		}
	}
	return false
}

// Retrieves the Peers configuration
func (c *PolicyConfigurationManager) getDiameterPeers() (DiameterPeers, error) {
	var peers []DiameterPeer
	peersMap := make(map[string]DiameterPeer)

	dp, err := c.CM.GetConfigObject("diameterPeers.json", true)
	if err != nil {
		return peersMap, err
	}
	json.Unmarshal(dp.RawBytes, &peers)

	// Cooking
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
	dp, error := c.getDiameterPeers()
	if error != nil {
		return fmt.Errorf("could not retrieve the Peers configuration: %w", error)
	}
	c.currentDiameterPeers = dp
	return nil
}

// Returs the current DiameterPeers configuration
func (c *PolicyConfigurationManager) PeersConf() DiameterPeers {
	return c.currentDiameterPeers
}
