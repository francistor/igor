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

	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testConfig"

	InitPolicyConfigInstance(bootFile, instanceName, true)
	InitHandlerConfigInstance(bootFile, instanceName, false)

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
			_, err := GetPolicyConfig().CM.GetConfigObject(objectName, true)
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
	dsc := GetPolicyConfig().DiameterServerConf()
	if dsc.BindAddress != "127.0.0.1" {
		t.Fatalf("BindAddress retreived is <%s>", dsc.BindAddress)
	}

	// Diameter Peers configuration
	dp := GetPolicyConfig().PeersConf()
	if dp["superserver.igorsuperserver"].WatchdogIntervalMillis != 300000 {
		t.Fatalf("WatchdogIntervalMillis was %d", dp["superserver.igorsuperserver"].WatchdogIntervalMillis)
	}
	peer, err := dp.FindPeer("client.igorclient")
	if err != nil {
		t.Fatalf("Peer not found for and origin-host client.igorclient")
	}
	if peer.DiameterHost != "client.igorclient" || peer.ConnectionPolicy != "passive" {
		t.Fatal("Found peer is not conforming to expected attributes", peer)
	}

	// Routing rules configuration
	rr := GetPolicyConfig().RoutingRulesConf()
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

func TestRadiusConfig(t *testing.T) {
	// Radius Server Configuration
	dsc := GetPolicyConfig().RadiusServerConf()
	if dsc.BindAddress != "0.0.0.0" {
		t.Errorf("Bind address was <%s>", dsc.BindAddress)
	}
	if dsc.OriginPorts[1] != 9001 {
		t.Errorf("Origin port 9001 not found")
	}

	// Radius Clients configuration
	rc := GetPolicyConfig().RadiusClientsConf()
	if rc["127.0.0.1"].Secret != "secret" {
		t.Errorf("secret for 127.0.0.1 is not as expeted")
	}

	// Get Radius Servers configuration
	rs := GetPolicyConfig().RadiusServersConf()
	if rs.Servers["non-existing-server"].IPAddress != "192.168.250.1" {
		t.Errorf("address of non-existing-server is not 192.168.250.1")
	}
	if rs.Servers["igor-superserver"].OriginPorts[0] != 8000 {
		t.Errorf("igor-superserver has unexpected origin port")
	}
	if rs.ServerGroups["igor-superserver-group"].Policy != "random" {
		t.Errorf("igor-supserserver server group has not policy random")
	}

	// Get Radius handlers configuration
	rh := GetPolicyConfig().RadiusHandlersConf()
	if rh.AuthHandlers[0] != "https://localhost:8080/radiusRequest" {
		t.Errorf("first radius handler for auth not as expected")
	}
}

func TestHandlerConfig(t *testing.T) {
	hc := GetHandlerConfig().HandlerConf()
	if hc.BindAddress != "0.0.0.0" {
		t.Fatalf("BindAddress was <%s>", hc.BindAddress)
	}
	if hc.BindPort != 8080 {
		t.Fatalf("BindPort was %d", hc.BindPort)
	}
	if hc.RouterIPAddress != "127.0.0.1" {
		t.Fatalf("RouterIPAddress was <%s>", hc.RouterIPAddress)
	}
	if hc.RouterPort != 23868 {
		t.Fatalf("RouterPort was %d", hc.RouterPort)
	}
}

// Retrieval of some JSON configuration file
func TestConfigFile(t *testing.T) {

	json, err := GetPolicyConfig().CM.GetConfigObjectAsJson("testFile.json", true)
	if err != nil {
		t.Fatal("Could not get configuration file testFile.json in \"testInstance\" folder", err)
	}
	var jsonMap = json.(map[string]interface{})
	if jsonMap["test"].(string) != "content" {
		t.Fatal("\"test\" property was not set to \"content\"")
	}
}
