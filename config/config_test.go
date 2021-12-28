package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	boot := "resources/searchRules.json"
	instance := "testInstance"
	Config.Init(boot, instance)

	os.Exit(m.Run())
}

// Diameter Configuration
func TestDiamConfig(t *testing.T) {

	// Diameter Server Configuration
	dsc, err := GetDiameterServerConfig()
	if err != nil {
		t.Fatal("Could not get Diameter Configuraton", err)
	}
	if dsc.BindAddress != "127.0.0.1" {
		t.Fatalf("Could not get BindAddress or was not %s", "127.0.0.1")
	}

	// Diameter Peers configuration
	dp, err := GetDiameterPeers()
	if err != nil {
		t.Fatal("Could not get Diameter Peers", err)
	}
	if dp[0].WatchdogIntervalMillis != 300000 {
		t.Fatal("WatchdogIntervalMillis was not 30000")
	}
	peer, err := dp.FindPeer("127.0.0.1", "client.igor")
	if err != nil {
		t.Fatalf("Peer not found for IP address %s and origin-host %s", "127.0.0.1", "client.igor")
	}
	if peer.OriginNetwork != "127.0.0.0/8" || peer.ConnectionPolicy != "passive" {
		t.Fatal("Found peer is not conforming to expected attributes", peer)
	}

	// Routing rules configuration
	rr, err := GetRoutingRules()
	if err != nil {
		t.Fatal("Could not get Routing Rules", err)
	}
	// Find the rule {"realm": "igorsuperserver", "applicationId": "*", "peers": ["superserver.igorsuperserver"], "policy": "fixed"}
	found := false
	for _, rule := range rr {
		if rule.Realm == "igorsuperserver" && rule.ApplicationId == "*" && rule.Peers[0] == "superserver.igorsuperserver" && rule.Policy == "fixed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Rule not found for {"realm": "igorsuperserver", "applicationId": "*", "peers": ["superserver.igorsuperserver"], "policy": "fixed"}`)
	}
	// Using the helper function
	rule, _ := rr.FindRoute("igorsuperserver", "Sp", false)
	if rule.Realm != "igorsuperserver" || rule.ApplicationId != "*" {
		t.Fatal(`Rule not found for realm "igorsuperserver" and applicaton "Sp"`)
	}
}

// Retrieval of some JSON configuration file
func TestConfigFile(t *testing.T) {

	json, err := Config.GetConfigObjectAsJSon("testFile.json")
	if err != nil {
		t.Fatal("Could not get configuration file testFile.json in \"testInstance\" folder", err)
	}
	var jsonMap = json.(map[string]interface{})
	if jsonMap["test"].(string) != "content" {
		t.Fatal("\"test\" property was not set to \"content\"")
	}

}
