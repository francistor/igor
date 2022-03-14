package config

import (
	"net/http"
	"os"
	"sync"
	"testing"
)

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir("resources"))
	http.Handle("/", fileHandler)
	err := http.ListenAndServe(":8100", nil)
	if err != nil {
		panic("could not start http server")
	}
}

func TestMain(m *testing.M) {

	// Initialize logger
	SetupLogger()

	// Initialize the Config Object as done in main.go
	bootFile := "resources/searchRules.json"
	instanceName := "testInstance"
	Config.Init(bootFile, instanceName)

	// Start the server for configuration
	go httpServer()

	os.Exit(m.Run())
}

// Retrieve a configuration object from multiple threads
func TestObjectRetrieval(t *testing.T) {

	var wg sync.WaitGroup

	var objectName = "testFile.json"
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Config.GetConfigObject(objectName)
			t.Log("Got configuration object")
			if err != nil {
				panic(err)
			}
		}()
	}
	wg.Wait()
}

// Diameter Configuration
func TestDiamConfig(t *testing.T) {

	// Diameter Server Configuration
	dsc := DiameterServerConf()
	if dsc.BindAddress != "127.0.0.1" {
		t.Fatalf("Could not get BindAddress or was not %s", "127.0.0.1")
	}

	// Diameter Peers configuration
	dp := PeersConf()
	if dp["superserver.igor"].WatchdogIntervalMillis != 300000 {
		t.Fatal("WatchdogIntervalMillis was not 30000")
	}
	peer, err := dp.FindPeer("127.0.0.1", "client.igor")
	if err != nil {
		t.Fatalf("Peer not found for IP address %s and origin-host %s", "127.0.0.1", "client.igor")
	}
	if peer.DiameterHost != "client.igor" || peer.ConnectionPolicy != "passive" {
		t.Fatal("Found peer is not conforming to expected attributes", peer)
	}

	// Routing rules configuration
	rr := RoutingRulesConf()
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
	rule, _ := rr.FindDiameterRoute("igorsuperserver", "Sp", false)
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
