package cdrwriter

import (
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/radiuscodec"
	"os"
	"strings"
	"testing"
	"time"
)

// Initialization
var bootstrapFile = "resources/searchRules.json"
var instanceName = "testClient"

// Initializer of the test suite.
func TestMain(m *testing.M) {
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestLivingstoneWriter(t *testing.T) {

	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1:1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name":"MyUserName"}
		]
	}`

	// Read JSON to Radius Packet
	rp := radiuscodec.RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, time.RFC3339, time.RFC3339)
	cdrString := lw.WriteCDRString(&rp)
	if strings.Contains(cdrString, "User-Name") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "Igor-InterfaceIdAttribute=\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}
}

func TestCSVWriter(t *testing.T) {

	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-StringAttribute": "anotherStringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1:1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name":"MyUserName"}
		]
	}`

	// Read JSON to Radius Packet
	rp := radiuscodec.RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	csvw := NewCSVWriter([]string{
		"Igor-OctetsAttribute",
		"Igor-StringAttribute",
		"Igor-IntegerAttribute",
		"Igor-AddressAttribute",
		"Igor-TimeAttribute",
		"Igor-IPv6AddressAttribute",
		"Igor-IPv6PrefixAttribute",
		"Igor-InterfaceIdAttribute",
		"Igor-Integer64Attribute",
		"Igor-SaltedOctetsAttribute"},
		";", ",", time.RFC3339, true)
	cdrString := csvw.WriteCDRString(&rp)
	if strings.Contains(cdrString, "MyUserName") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}

	fmt.Println(cdrString)
}
