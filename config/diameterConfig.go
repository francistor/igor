package config

import (
	"encoding/json"
	"fmt"
	"net"
)

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
}

// Retrieves the diameter server configuration
func (c *ConfigurationManager) getDiameterServerConfig() (DiameterServerConfig, error) {
	dsc := DiameterServerConfig{}
	dc, err := c.GetConfigObject("diameterServer.json")
	if err != nil {
		return dsc, err
	}
	json.Unmarshal([]byte(dc.RawText), &dsc)
	return dsc, nil
}

func (c *ConfigurationManager) UpdateDiameterServerConfig() error {
	dsc, error := c.getDiameterServerConfig()
	if error != nil {
		c.IgorLogger.Errorf("could not retrieve the Diameter Server configuration: %v", error)
		return error
	}
	c.currentDiameterServerConfig = dsc
	return nil
}

func (c *ConfigurationManager) DiameterServerConf() DiameterServerConfig {
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
func (c *ConfigurationManager) getDiameterRoutingRules() (DiameterRoutingRules, error) {
	var routingRules []DiameterRoutingRule
	rr, err := c.GetConfigObject("diameterRoutes.json")
	if err != nil {
		return routingRules, err
	}
	json.Unmarshal([]byte(rr.RawText), &routingRules)
	return routingRules, nil
}

func (c *ConfigurationManager) UpdateDiameterRoutingRules() {
	drr, error := c.getDiameterRoutingRules()
	if error != nil {
		c.IgorLogger.Errorf("could not retrieve the Diameter Rules configuration: %v", error)
		return
	}
	c.currentRoutingRules = drr
}

func (c *ConfigurationManager) RoutingRulesConf() DiameterRoutingRules {
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
func (c *ConfigurationManager) getDiameterPeers() (DiameterPeers, error) {
	var peers []DiameterPeer
	peersMap := make(map[string]DiameterPeer)

	dp, err := c.GetConfigObject("diameterPeers.json")
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

func (c *ConfigurationManager) UpdateDiameterPeers() {
	dp, error := c.getDiameterPeers()
	if error != nil {
		c.IgorLogger.Errorf("could not retrieve the Peers configuration: %v", error)
		return
	}
	c.currentDiameterPeers = dp
}

func (c *ConfigurationManager) PeersConf() DiameterPeers {
	return c.currentDiameterPeers
}
