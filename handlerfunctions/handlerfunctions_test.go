package handlerfunctions

import (
	"bytes"
	"encoding/json"
	"igor/config"
	"igor/radiuscodec"
	"os"
	"testing"
)

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testConfig"

	config.InitPolicyConfigInstance(bootFile, instanceName, true)

	os.Exit(m.Run())
}

func TestRadiusUserFile(t *testing.T) {

	ruf, err := NewRadiusUserFile("radiusUserFile.json", config.GetPolicyConfigInstance("testConfig"))
	if err != nil {
		t.Fatal(err)
	}

	if ruf["key1"].CheckItems["clientType"] != "client-type-1" {
		t.Fatal("bad check item value")
	}

	if ruf["key1"].ReplyItems[2].GetInt() != 1 {
		t.Fatalf("bad reply item value %d", ruf["key1"].ReplyItems[2].GetInt())
	}

	if ruf["key1"].NonOverridableReplyItems[0].GetString() != "a=b" {
		t.Fatalf("bad non overridable item value %s", ruf["key1"].NonOverridableReplyItems[0].GetString())
	}

	if ruf["key1"].OOBReplyItems[0].GetString() != "service-info-value" {
		t.Fatalf("bad non overridable item value %s", ruf["key1"].OOBReplyItems[0].GetString())
	}

	//jEntry, err := json.Marshal(entry)

	//fmt.Println(PrettyPrintJSON(jEntry))
}

func TestRadiusChecks(t *testing.T) {

	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-TaggedStringAttribute": "myString:1"},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name":"MyUserName"}
		]
	}`

	// Read JSON to Radius Packet
	rp := radiuscodec.RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	radiusChecks, err := NewRadiusChecks("radiusChecks.json", config.GetPolicyConfigInstance("testConfig"))
	if err != nil {
		t.Fatalf("error parsing radiusCheck.json: %s", err.Error())
	}

	// Valid check
	if !radiusChecks.CheckPacket("myCheck", &rp) {
		t.Fatalf("wrongly discarded packet")
	}

	// Remove one attribute, so that the check is not valid anymore
	rp.DeleteAllAVP("Igor-SaltedOctetsAttribute")
	if radiusChecks.CheckPacket("myCheck", &rp) {
		t.Fatalf("wrongly accepted packet")
	}

	// Check with branch only
	if !radiusChecks.CheckPacket("leafOnlyCheck1", &rp) {
		t.Fatalf("wrongly discarded packet")
	}
	if radiusChecks.CheckPacket("leafOnlyCheck2", &rp) {
		t.Fatalf("wrongly accepted packet")
	}
}

func TestRadiusFilters(t *testing.T) {

	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-TaggedStringAttribute": "myString:1"},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name":"MyUserName"}
		]
	}`

	// Read JSON to Radius Packet
	rp := radiuscodec.RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	filters, err := NewAVPFilters("avpFilters.json", nil)
	if err != nil {
		t.Fatalf("error reading avpFilters.json")
	}

	frp, err := filters.FilterPacket("myFilter", &rp)
	if err != nil {
		t.Fatalf("error reading filters file")
	}
	if frp.GetStringAVP("Igor-OctetsAttibute") != "" {
		t.Fatalf("attribute not removed")
	}
	if frp.GetStringAVP("User-Name") != "Modified-User-Name" {
		t.Fatalf("attribute not modified")
	}
}

// Helper to show JSON to humans
func PrettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
