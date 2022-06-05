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

	// Rules
	policyConfig.CM.FillSearchRules(bootstrapFile)

	// Initialize logger and dictionary, if default
	if isDefault {
		initLogger(&policyConfig.CM)
		initDictionaries(&policyConfig.CM)
	}

	// Load diameter configuraton
	policyConfig.UpdateDiameterServerConfig()
	policyConfig.UpdateDiameterPeers()
	policyConfig.UpdateDiameterRoutingRules()

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
	PeerCheckTimeSeconds string
	HttpBindAddress      string
	HttpBindPort         int
}

// Retrieves the diameter server configuration
func (c *PolicyConfigurationManager) getDiameterServerConfig() (DiameterServerConfig, error) {
	dsc := DiameterServerConfig{}
	dc, err := c.CM.GetConfigObject("diameterServer.json")
	if err != nil {
		return dsc, err
	}
	json.Unmarshal([]byte(dc.RawText), &dsc)
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

type DiameterRoutingRule struct {
	Realm         string
	ApplicationId string
	Handlers      []string // URL to send the request to
	Peers         []string // Peers to send the request to (handler should be empty)
	Policy        string   // May be "fixed" or "random"
}

type DiameterRoutingRules []DiameterRoutingRule

// Finds the appropriate route, taking into account wildcards.
// If nonLocal is true, force that the Router is not local (has no nandler)
func (rr DiameterRoutingRules) FindDiameterRoute(realm string, application string, nonLocal bool) (DiameterRoutingRule, error) {
	for _, rule := range rr {
		if rule.Realm == "*" || rule.Realm == realm {
			if rule.ApplicationId == "*" || rule.ApplicationId == application {
				if !nonLocal || (nonLocal && len(rule.Handlers) == 0) {
					return rule, nil
				}
			}
		}
	}

	return DiameterRoutingRule{}, fmt.Errorf("rule not found for realm %s, application %s, nonLocal: %t", realm, application, nonLocal)
}

// Retrieves the Routes configuration
func (c *PolicyConfigurationManager) getDiameterRoutingRules() (DiameterRoutingRules, error) {
	var routingRules []DiameterRoutingRule
	rr, err := c.CM.GetConfigObject("diameterRoutes.json")
	if err != nil {
		return routingRules, err
	}
	json.Unmarshal([]byte(rr.RawText), &routingRules)
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

	dp, err := c.CM.GetConfigObject("diameterPeers.json")
	if err != nil {
		return peersMap, err
	}
	json.Unmarshal([]byte(dp.RawText), &peers)

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
