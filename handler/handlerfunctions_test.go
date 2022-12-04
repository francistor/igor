package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/radiuscodec"
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

	if ruf["key1"].ConfigItems["clientType"] != "client-type-1" {
		t.Fatal("bad config item value")
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

func TestMergeUserFileEntry(t *testing.T) {
	ruf1, err := NewRadiusUserFile("radiusUserFileHighPriority.json", config.GetPolicyConfigInstance("testConfig"))
	if err != nil {
		t.Fatal(err)
	}

	ruf2, err := NewRadiusUserFile("radiusUserFileHighPriority.json", config.GetPolicyConfigInstance("testConfig"))
	if err != nil {
		t.Fatal(err)
	}

	pEntries := ruf1["key1"].ConfigItems.OverrideWith(ruf2["key1"].ConfigItems)
	if pEntries["clientType"] != "client-type-2" {
		t.Fatal("bad overriden entry for clientType")
	}
	if pEntries["additionalItem"] != "additional" {
		t.Fatal("bad overriden entry for additionalItem")
	}

	ovEntries := ruf1["key1"].ReplyItems.OverrideWith(ruf2["key1"].ReplyItems)
	classAttrs := findAttributes(ovEntries, "Class")
	if len(classAttrs) != 1 {
		t.Fatal("number of class attributes is not 1")
	}
	if classAttrs[0].GetString() != "theClassAttribute2" {
		t.Fatal("bad merged entry for Class")
	}
	stringAttribute := findAttributes(ovEntries, "Igor-StringAttribute")
	if len(stringAttribute) != 1 {
		t.Fatal("number of class stringAttribute is not 1")
	}
	if stringAttribute[0].GetString() != "additional" {
		t.Fatal("bad merged entry for StringAttribute")
	}

	noEntries := ruf1["key1"].NonOverridableReplyItems.Add(ruf2["key1"].NonOverridableReplyItems)
	ciscoAVPAttributes := findAttributes(noEntries, "Cisco-AVPair")
	if len(ciscoAVPAttributes) != 2 {
		t.Fatalf("number of class ciscoAVPAttributes is not 2 but %d", len(ciscoAVPAttributes))
	}
	if ciscoAVPAttributes[1].GetString() != "c=d" {
		t.Fatal("bad merged entry for Cisco-AVPair")
	}
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

	radiusChecks, err := NewRadiusPacketChecks("radiusChecks.json", config.GetPolicyConfigInstance("testConfig"))
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
	if !radiusChecks.CheckPacket("leafOnlyUserNameMatchesMy", &rp) {
		t.Fatalf("wrongly discarded packet")
	}
	if radiusChecks.CheckPacket("leafOnlyUserNameMatchesOnly", &rp) {
		t.Fatalf("wrongly accepted packet")
	}
	if radiusChecks.CheckPacket("leafOnlyCheckClassPresent", &rp) {
		t.Fatalf("wrongly accepted packet (Class)")
	}
	if !radiusChecks.CheckPacket("leafOnlyCheckClassNotPresent", &rp) {
		t.Fatalf("wrongly rejected packet (Class)")
	}
	if radiusChecks.CheckPacket("leafOnlyCheckUserNameNotPresent", &rp) {
		t.Fatalf("wrongly accepted packet (User-Name)")
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

	filters, err := NewAVPFilters("radiusFilters.json", nil)
	if err != nil {
		t.Fatalf("error reading radiusFilters.json")
	}

	frp, err := filters.FilteredPacket(&rp, "myFilter")
	if err != nil {
		t.Fatalf("error reading filters file")
	}

	if frp.GetStringAVP("Igor-OctetsAttribute") != "" {
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

// Helper to look for AVP in an slice
func findAttributes(v []radiuscodec.RadiusAVP, name string) []radiuscodec.RadiusAVP {
	avps := make([]radiuscodec.RadiusAVP, 0)
	for _, avp := range v {
		if avp.Name == name {
			avps = append(avps, avp)
		}
	}

	return avps
}

func TestTemplatedConfigObject(t *testing.T) {
	type CParam struct {
		Speed   int
		Message string
	}
	// To avoid warning
	var v CParam
	println(v)

	var o = config.NewTemplatedConfigObject[RadiusUserFile, CParam]("template.txt", "templateParameters.json")
	if err := o.Update(&config.GetPolicyConfig().CM); err != nil {
		t.Fatalf("could not get templated configuration object: %s", err)
	}

	fmt.Printf("%#v\n", o.Get())

}
