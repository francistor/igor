package config

import (
	"encoding/json"
	"fmt"
)

type DiameterServerConfig struct {
	BindAddress          string
	BindPort             int
	DiameterHost         string
	DiameterRealm        string
	VendorId             int
	ProductName          string
	FirmwareRevision     string
	PeerCheckTimeSeconds string
}

type RoutingRule struct {
	Realm         string
	ApplicationId string
	Handler       string   // If has a handler, will be treated locally
	Peers         []string // Peers to send the request to (handler should be empty)
	Policy        string   // May be "fixed" or "random"
}

type RoutingRules []RoutingRule

// Finds the appropriate route, taking into account wildcards.
// If nonLocal is true, force that the router is not local (has no nandler)
func (rr RoutingRules) FindRoute(realm string, application string, nonLocal bool) (RoutingRule, error) {
	for _, rule := range rr {
		if rule.Realm == "*" || rule.Realm == realm {
			if rule.ApplicationId == "*" || rule.ApplicationId == application {
				if !nonLocal || (nonLocal && rule.Handler == "") {
					return rule, nil
				}
			}
		}
	}

	return RoutingRule{}, fmt.Errorf("rule not found for realm %s, application %s, nonLocal: %t", realm, application, nonLocal)
}

type DiameterPeer struct {
	DiameterHost           string
	IPAddress              string
	Port                   int
	ConnectionPolicy       string // May be "active" or "passive"
	OriginNetwork          string // CIDR
	WatchdogIntervalMillis int
}

type DiameterPeers []DiameterPeer

// Gets the first Diameter Peer that conforms to the specification: IPAddress and Origin-Host reported
func (dps DiameterPeers) FindPeer(remoteIPAddress string, diameterHost string) (DiameterPeer, error) {

	for _, peer := range dps {
		if peer.IPAddress == remoteIPAddress && peer.DiameterHost == diameterHost {
			return peer, nil
		}
	}

	return DiameterPeer{}, fmt.Errorf("no Peer found for IPAddress %s and origin-host %s", remoteIPAddress, diameterHost)
}

type Handler struct {
	Name    string
	Handler string
}

type Handlers []Handler

// Retrieves the diameter server configuration
func GetDiameterServerConfig() (DiameterServerConfig, error) {
	dsc := DiameterServerConfig{}
	dc, err := Config.GetConfigObject("diameterServer.json")
	if err != nil {
		return dsc, err
	}
	json.Unmarshal([]byte(dc.RawText), &dsc)
	return dsc, nil
}

// Retrieves the Peers configuration
func GetDiameterPeers() (DiameterPeers, error) {
	var peers []DiameterPeer
	dp, err := Config.GetConfigObject("diameterPeers.json")
	if err != nil {
		return peers, err
	}
	json.Unmarshal([]byte(dp.RawText), &peers)
	return peers, nil
}

// Retrieves the Routes configuration
func GetRoutingRules() (RoutingRules, error) {
	var routes []RoutingRule
	rr, err := Config.GetConfigObject("diameterRoutes.json")
	if err != nil {
		return routes, err
	}
	json.Unmarshal([]byte(rr.RawText), &routes)
	return routes, nil
}
