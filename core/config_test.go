package core

import (
	"net"
	"strings"
	"testing"
)

func TestHttpRetrieval(t *testing.T) {
	txt, err := GetPolicyConfig().CM.GetBytesConfigObject("template_http.txt")
	if err != nil {
		t.Fatalf("error using http to get config object: %s", err)
	}
	if !strings.Contains(string(txt), "internet") {
		t.Fatal("contents of http object are not ok")
	}
}

// Uncomment to test
// Requires to populate the PSBA database with
// INSERT INTO accessNodes (AccessNodeId, Parameters) values ("RepublicaHW01", '{"ipAddress": "127.0.0.1", "secret": "mysecret", "attributes": [{"Redback-Client_DNS_Pri": "1.2.3.4"}, {"Session-Timeout": 3600}]}');
// INSERT INTO accessNodes (AccessNodeId, Parameters) values ("RepublicaHW02", '{"ipAddress": "127.0.0.2", "secret": "mysecret", "attributes": [{"Redback-Client_DNS_Pri": "1.2.3.4"}, {"Session-Timeout": 7200}]}');
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

	if rcEntries["127.0.0.1"].Secret != "mysecret" {
		t.Fatalf("bad content in radiusclients.database")
	}
}

func TestParametrizedObject(t *testing.T) {
	type TestObject struct {
		MyString  string
		MyInteger int
	}

	co := NewConfigObject[TestObject]("parametrized.json.templ")
	err := co.Update(&GetPolicyConfig().CM)
	if err != nil {
		t.Fatal("could not retrieve parametrized.templ")
	}

	// Values for this are set as environment variables at initialization of testing
	if co.Get().MyString != igor_string_parameter {
		t.Fatal("parametrized string object did not have the expected value")
	}

	if co.Get().MyInteger != igor_int_parameter {
		t.Fatal("parametrized integer object did not have the expected value")
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
	dp := GetPolicyConfig().DiameterPeers()
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
	rr := GetPolicyConfig().DiameterRoutingRules()
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
	rule, _ := rr.FindDiameterRoutingRule("igorsuperserver", "Sp", false)
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
	rc := GetPolicyConfig().RadiusClients()
	if rc["127.0.0.1"].Secret != "secret" {
		t.Fatalf("secret for 127.0.0.1 is not as expeted")
	}
	if rc["127.0.0.1"].ClientProperties["scope"] != "default" {
		t.Fatalf("property for scope not ok")
	}
	if rc["127.0.0.1"].IPAddress != "127.0.0.1" {
		t.Fatalf("IPAddress was not in the radius client object")
	}
	if rc["127.0.0.1"].RadiusAttributes[0].GetInt() != 120 {
		t.Fatalf("Bad Session-Timeout radius attribute")
	}

	// Find radius client
	_, err := rc.FindRadiusClient(net.ParseIP("1.2.3.4"))
	if err != nil {
		t.Fatalf("Radius client 1.2.3.4 not found")
	}

	// Get Radius Servers configuration
	rs := GetPolicyConfig().RadiusServers()
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
	rh := GetPolicyConfig().RadiusHttpHandlers()
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
