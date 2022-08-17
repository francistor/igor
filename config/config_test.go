package config

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir("resources"))
	http.Handle("/", fileHandler)
	if err := http.ListenAndServe(":8100", nil); err != nil {
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

	var co *ConfigObject
	var err error
	var objectName = "testFile.json"
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			co, err = GetPolicyConfig().CM.GetConfigObject(objectName, true)
			t.Log("Got configuration object")
			if err != nil {
				panic(err)
			}
		}()
	}
	wg.Wait()

	// Parse Raw
	if !strings.Contains(string(co.RawBytes), "correctly") {
		t.Fatal("raw testFile.json not retrieved correctly")
	}

	// Parse as JSON
	var jsonMap = co.Json.(map[string]interface{})
	if jsonMap["test"].(string) != "file retreived correctly" {
		t.Fatal("json testFile.json not retrieved correctly")
	}
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
	peer, found := dp["client.igorclient"]
	if !found {
		t.Fatalf("Peer not found for client.igorclient")
	}
	if peer.DiameterHost != "client.igorclient" || peer.ConnectionPolicy != "passive" {
		t.Fatal("Found peer is not conforming to expected attributes", peer)
	}

	// Routing rules configuration
	// Find the rule {"realm": "igorsuperserver", "applicationId": "*", "peers": ["superserver.igorsuperserver"], "policy": "fixed"}
	rr := GetPolicyConfig().RoutingRulesConf()
	found = false
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
		t.Fatalf("Bind address was <%s>", dsc.BindAddress)
	}
	if dsc.OriginPorts[1] != 9001 {
		t.Fatalf("Origin port 9001 not found")
	}

	// Radius Clients configuration
	rc := GetPolicyConfig().RadiusClientsConf()
	if rc["127.0.0.1"].Secret != "secret" {
		t.Fatalf("secret for 127.0.0.1 is not as expeted")
	}

	// Get Radius Servers configuration
	rs := GetPolicyConfig().RadiusServersConf()
	if rs.Servers["non-existing-server"].IPAddress != "127.0.0.2" {
		t.Fatalf("address of non-existing-server is not 127.0.0.2")
	}
	if rs.Servers["igor-superserver"].OriginPorts[0] != 8000 {
		t.Fatalf("igor-superserver has unexpected origin port")
	}
	if rs.ServerGroups["igor-superserver-group"].Policy != "random" {
		t.Fatalf("igor-supserserver server group has not policy random")
	}

	// Get Radius handlers configuration
	rh := GetPolicyConfig().RadiusHandlersConf()
	if rh.AuthHandlers[0] != "https://localhost:8080/radiusRequest" {
		t.Fatalf("first radius handler for auth not as expected")
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
	if hc.RouterAddress != "127.0.0.1" {
		t.Fatalf("RouterAddress was <%s>", hc.RouterAddress)
	}
	if hc.RouterPort != 20000 {
		t.Fatalf("RouterPort was %d", hc.RouterPort)
	}
}

func TestHttpRouterConfig(t *testing.T) {
	hrc := GetPolicyConfig().HttpRouterConf()
	if hrc.BindAddress != "0.0.0.0" {
		t.Fatalf("BindAddress was <%s>", hrc.BindAddress)
	}
	if hrc.BindPort != 20000 {
		t.Fatalf("BindPort was %d", hrc.BindPort)
	}
}
