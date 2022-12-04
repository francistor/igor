package config

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir(os.Getenv("IGOR_BASE") + "resources"))
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
	InitHttpHandlerConfigInstance(bootFile, instanceName, false)

	// Start the server for configuration
	go httpServer()

	os.Exit(m.Run())
}

// Uncomment to test
// Requires to populate the PSBA database with
// INSERT INTO accessNodes (AccessNodeId, Parameters) values ("RepublicaHW01", '{"ipAddress": "127.0.0.1", "secret": "mysecret", "attributes": [{"Redback-Primary-DNS": "1.2.3.4"}, {"Session-Timeout": 3600}]}');
// INSERT INTO accessNodes (AccessNodeId, Parameters) values ("RepublicaHW02", '{"ipAddress": "127.0.0.2", "secret": "mysecret", "attributes": [{"Redback-Primary-DNS": "1.2.3.4"}, {"Session-Timeout": 7200}]}');
func TestDatabaseObject(t *testing.T) {

	type RadiusClientEntry struct {
		Secret    string
		IPAddress string
	}

	var rcEntries map[string]RadiusClientEntry
	err := GetPolicyConfig().CM.BuildJSONConfigObject("radiusclients.database", &rcEntries)
	if err != nil {
		t.Fatalf("could not read radiusclients.database: %s", err)
	}

	if rcEntries["RepublicaHW01"].Secret != "mysecret" {
		t.Fatalf("bad content in radiusclients.database")
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
	if dp["superserver.igorsuperserver"].DiameterHost != "superserver.igorsuperserver" {
		t.Fatal("DiameterHost was not found for superserver.igorsuperserver")
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
	if rc["127.0.0.1"].ClientProperties["scope"] != "default" {
		t.Fatalf("property for scope not ok")
	}
	if rc["127.0.0.1"].ClientProperties["scope"] != "default" {
		t.Fatalf("property for scope not ok")
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
	rh := GetPolicyConfig().RadiusHttpHandlersConf()
	if rh.AuthHandlers[0] != "https://localhost:8080/radiusRequest" {
		t.Fatalf("first radius handler for auth not as expected")
	}
}

func TestHttpHandlerConfig(t *testing.T) {
	hc := GetHttpHandlerConfig().HttpHandlerConf()
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

func TestHandlerLogger(t *testing.T) {

	hl := NewHandlerLogger()
	hl.L.Debugf("message in debug level <%s>", "debug message")
	hl.L.Infof("message in info level <%s>", "info message")
	logDump := hl.String()
	if strings.Contains(logDump, "<debug message>") {
		t.Fatalf("incorrectly dumped handler logger message in debug mode")
	}
	if !strings.Contains(logDump, "<info message>") {
		t.Fatalf("missing handler logger message in info mode")
	}
}

func TestParametricObject(t *testing.T) {
	type CParam struct {
		Speed   int
		Message string
	}

	oBytes, err := GetBytesTemplatedConfigObject[CParam]("template.txt", "templateParameters.json", nil)
	if err != nil {
		t.Fatalf("error in getting templated config object %s", err)
	}

	var o map[string]interface{}
	err = json.Unmarshal(oBytes, &o)
	if err != nil {
		t.Fatalf("error in unmarshaling templated config object %s", err)
	}

	okey1 := o["key1"].(map[string]interface{})
	okey1Internet := okey1["internet"].(map[string]interface{})
	okey1InternetReplyItems := okey1Internet["replyItems"].([]interface{})
	if len(okey1InternetReplyItems) != 2 {
		t.Fatalf("number of reply items is incorrect")
	}
}
