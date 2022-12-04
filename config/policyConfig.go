package config

import (
	"fmt"
	"net"
)

// Manages the configuration items for policy
type PolicyConfigurationManager struct {
	CM ConfigurationManager

	diameterServerConfig *ConfigObject[DiameterServerConfig]
	diameterRoutes       *ConfigObject[DiameterRoutingRules]
	diameterPeers        *ConfigObject[DiameterPeers]

	radiusServerConfig *ConfigObject[RadiusServerConfig]
	radiusClients      *ConfigObject[RadiusClients]
	radiusServers      *ConfigObject[RadiusServers]
	radiusHttpHandlers *ConfigObject[RadiusHttpHandlers]

	httpRouterConfig *ConfigObject[HttpRouterConfig]
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
	policyConfig := PolicyConfigurationManager{
		CM:                   NewConfigurationManager(bootstrapFile, instanceName),
		diameterServerConfig: NewConfigObject[DiameterServerConfig]("diameterServer.json"),
		diameterRoutes:       NewConfigObject[DiameterRoutingRules]("diameterRoutes.json"),
		diameterPeers:        NewConfigObject[DiameterPeers]("diameterPeers.json"),
		radiusServerConfig:   NewConfigObject[RadiusServerConfig]("radiusServer.json"),
		radiusServers:        NewConfigObject[RadiusServers]("radiusServers.json"),
		radiusClients:        NewConfigObject[RadiusClients]("radiusClients.json"),
		radiusHttpHandlers:   NewConfigObject[RadiusHttpHandlers]("radiusHttpHandlers.json"),
		httpRouterConfig:     NewConfigObject[HttpRouterConfig]("httpRouter.json"),
	}
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
	if policyConfig.diameterServerConfig.Get().BindAddress != "" {
		if cerr = policyConfig.UpdateDiameterPeers(); cerr != nil {
			panic(cerr)
		}
		if cerr = policyConfig.UpdateDiameterRoutingRules(); cerr != nil {
			panic(cerr)
		}
	} else {
		GetLogger().Info("diameter server not configured")
	}

	// Load radius configuration
	if cerr = policyConfig.UpdateRadiusServerConfig(); cerr != nil {
		panic(cerr)
	}
	if policyConfig.radiusServerConfig.Get().BindAddress != "" {
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
		GetLogger().Info("radius server not configured")
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
	return c.diameterServerConfig.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the diameter server configuration
func (c *PolicyConfigurationManager) DiameterServerConf() DiameterServerConfig {
	return c.diameterServerConfig.Get()
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
	return c.radiusServerConfig.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the radius server configuration
func (c *PolicyConfigurationManager) RadiusServerConf() RadiusServerConfig {
	return c.radiusServerConfig.Get()
}

// Holds the configuration of a Radius Client
// Key in the RadiusClients map will be the IPAddress
type RadiusClient struct {
	Name             string
	IPAddress        string
	Secret           string
	ClientClass      string
	ClientProperties map[string]string
}

// Holds the configuration of all Radius Clients, indexed by IP address
type RadiusClients map[string]RadiusClient

// Initializer to copy the name into the RadiusClient struct
func (rc RadiusClients) initialize() error {
	for ipAddr, client := range rc {
		client.IPAddress = ipAddr
		rc[ipAddr] = client
	}

	return nil
}

// Updates the radius clients configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusClients() error {
	return c.radiusClients.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the radius clients configuration
func (c *PolicyConfigurationManager) RadiusClientsConf() RadiusClients {
	return c.radiusClients.Get()
}

// Holds the configuration for an upstream Radius Server
type RadiusServer struct {
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
	return c.radiusServers.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the radius servers configuration
func (c *PolicyConfigurationManager) RadiusServersConf() RadiusServers {
	return c.radiusServers.Get()
}

// Holds the radius handlers configuration
type RadiusHttpHandlers struct {
	AuthHandlers []string
	AcctHandlers []string
	COAHandlers  []string
}

// Updates the radius handlers configuration in the global variable
func (c *PolicyConfigurationManager) UpdateRadiusHttpHandlers() error {
	return c.radiusHttpHandlers.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the radius handlers configuration
func (c *PolicyConfigurationManager) RadiusHttpHandlersConf() RadiusHttpHandlers {
	return c.radiusHttpHandlers.Get()
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
	return c.diameterRoutes.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the diameter routing rules configuration
func (c *PolicyConfigurationManager) RoutingRulesConf() DiameterRoutingRules {
	return c.diameterRoutes.Get()
}

///////////////////////////////////////////////////////////////////////////////

// Holds the configuration of a Diameter Peer
type DiameterPeer struct {
	IPAddress               string
	Port                    int
	ConnectionPolicy        string // May be "active" or "passive"
	OriginNetwork           string // CIDR
	WatchdogIntervalMillis  int
	ConnectionTimeoutMillis int

	// Cooked
	OriginNetworkCIDR net.IPNet
	DiameterHost      string
}

// Holds the configuration of all Diameter peers
type DiameterPeers map[string]DiameterPeer

// Implements the Initializable interface
// Performs the cooking of the just read configuration object
func (dps DiameterPeers) initialize() error {
	// Adding parsed origin network and diameter host
	for dHost, peer := range dps {
		_, ipNet, err := net.ParseCIDR(peer.OriginNetwork)
		if err != nil {
			return fmt.Errorf("could not retrieve the Diameter Peers configuration. Bad origin address %s", peer.OriginNetwork)
		}
		peer.OriginNetworkCIDR = *ipNet
		peer.DiameterHost = dHost
		dps[dHost] = peer
	}

	return nil
}

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
	return c.diameterPeers.Update(&c.CM)
}

// Returs the current DiameterPeers configuration
func (c *PolicyConfigurationManager) PeersConf() DiameterPeers {
	return c.diameterPeers.Get()
}

///////////////////////////////////////////////////////////////////////////////

// Holds the configuration fot the HTTP Router
type HttpRouterConfig struct {
	BindAddress string
	BindPort    int
}

// Updates the diameter server configuration in the global variable
func (c *PolicyConfigurationManager) UpdateHttpRouterConfig() error {
	return c.httpRouterConfig.Update(&c.CM)
}

// Retrieves the contents of the global variable containing the diameter server configuration
func (c *PolicyConfigurationManager) HttpRouterConf() HttpRouterConfig {
	return c.httpRouterConfig.Get()
}
