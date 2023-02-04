package cdrwriter

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/francistor/igor/core"
)

// Initialization
var bootstrapFile = "resources/searchRules.json"
var instanceName = "testConfig"
var cdrDirectoryName = "../cdr"

var jElasticConfig = `
{
	"attributeMap": {
		"IgorOctets": "Igor-OctetsAttribute",
		"IgorString": "Igor-StringAttribute",
		"SessionTime": "Acct-Session-Time!Acct-Delay-Time",
		"InputBytes": "Acct-Input-Octets<Acct-Input-Gigawords",
		"Status": "Acct-Status-Type"
	},
	"indexName": "cdr-",
	"indexType": "field",
	"attributeForIndex": "Igor-TimeAttribute",
	"indexDateFormat": "2006-01",
	"idFields": ["Acct-Session-Id", "NAS-IP-Address"],
	"versionField": "Igor-TimeAttribute"
}`

// Initializer of the test suite.
func TestMain(m *testing.M) {
	core.InitPolicyConfigInstance(bootstrapFile, instanceName, nil, true)

	// Execute the tests
	exitCode := m.Run()

	// Clean cdr files
	os.RemoveAll(cdrDirectoryName)

	os.Exit(exitCode)
}

func TestLivingstoneFormat(t *testing.T) {

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, "2006-01-02T15:04:05 MST", "2006-01-02T15:04:05 MST")
	cdrString := lw.GetRadiusCDRString(&rp)
	if strings.Contains(cdrString, "User-Name") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "Igor-InterfaceIdAttribute=\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}
}

func TestCSVFormat(t *testing.T) {

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	result := `"0102030405060708090a0b";;"stringvalue,anotherStringvalue";0,1,1;"127.0.0.1";"1986-11-26T03:34:08 UTC";"bebe:cafe::";"bebe:cafe:cccc::0/64";"00aabbccddeeff11";999999999999;"myString:1";"1122aabbccdd"`
	csvw := NewCSVWriter([]string{
		"Igor-OctetsAttribute",
		"Non-existing",
		"Igor-StringAttribute",
		"Igor-IntegerAttribute",
		"Igor-AddressAttribute",
		"Igor-TimeAttribute",
		"Igor-IPv6AddressAttribute",
		"Igor-IPv6PrefixAttribute",
		"Igor-InterfaceIdAttribute",
		"Igor-Integer64Attribute",
		"Igor-TaggedStringAttribute",
		"Igor-SaltedOctetsAttribute"},
		";", ",", "2006-01-02T15:04:05 MST", true, false)

	cdrString := csvw.GetRadiusCDRString(&rp)
	if strings.Contains(cdrString, "MyUserName") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}
	if !strings.Contains(cdrString, ";;") {
		t.Fatalf("pattern for non existing attribute not found")
	}
	if cdrString != result+"\n" {
		t.Fatalf("bad csv string <%s>", cdrString)
	}
}

func TestCSVParsedIntsFormat(t *testing.T) {

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	result := `"0102030405060708090a0b";;"stringvalue,anotherStringvalue";"Zero,One,One";"127.0.0.1";"1986-11-26T03:34:08 UTC";"bebe:cafe::";"bebe:cafe:cccc::0/64";"00aabbccddeeff11";"999999999999";"myString:1";"1122aabbccdd"`
	csvw := NewCSVWriter([]string{
		"%Timestamp%",
		"Igor-OctetsAttribute",
		"Non-existing",
		"Igor-StringAttribute",
		"Igor-IntegerAttribute",
		"Igor-AddressAttribute",
		"Igor-TimeAttribute",
		"Igor-IPv6AddressAttribute",
		"Igor-IPv6PrefixAttribute",
		"Igor-InterfaceIdAttribute",
		"Igor-Integer64Attribute",
		"Igor-TaggedStringAttribute",
		"Igor-SaltedOctetsAttribute"},
		";", ",", "2006-01-02T15:04:05 MST", true, true)

	cdrString := csvw.GetRadiusCDRString(&rp)
	if strings.Contains(cdrString, "MyUserName") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}
	if !strings.Contains(cdrString, ";;") {
		t.Fatalf("pattern for non existing attribute not found")
	}
	if !strings.Contains(cdrString, result) {
		t.Fatalf("bad csv string <%s>", cdrString)
	}

	t.Log(cdrString)
}

func TestJSONFormat(t *testing.T) {

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewJSONWriter(nil, []string{"User-Name"})
	cdrString := lw.GetRadiusCDRString(&rp)
	if strings.Contains(cdrString, "User-Name") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "\"Igor-InterfaceIdAttribute\":\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written json")
	}
}

func TestElasticFormat(t *testing.T) {

	var conf ElasticWriterConf
	if err := json.Unmarshal([]byte(jElasticConfig), &conf); err != nil {
		t.Fatalf("bad ElasticWriterConf format: %s", err)
	}
	ew := NewElasticWriter(conf)

	rp := buildSimpleRadiusPacket(t)

	// Accounting start
	rp.Add("Acct-Status-Type", "Stop")
	esCDR := ew.GetRadiusCDRString(&rp)

	if !strings.Contains(esCDR, "\"_index\": \"cdr-1986-11\"") {
		t.Fatal("bad index name")
	}
	if !strings.Contains(esCDR, "\"_id\": \"session-1|127.0.0.1|\"") {
		t.Fatal("bad _id")
	}
	if !strings.Contains(esCDR, "\"version\": 200533360048") {
		t.Fatal("bad version")
	}
	if !strings.Contains(esCDR, "\"InputBytes\": 4294968296") {
		t.Fatal("bad input bytes")
	}
	if !strings.Contains(esCDR, "\"Status\": \"Stop\"") {
		t.Fatal("bad accounting status type")
	}
}

func TestFileWriter(t *testing.T) {

	// For being able to execute a single test
	os.RemoveAll(cdrDirectoryName)

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, "2006-01-02T15:04:05 MST", "2006-01-02T15:04:05 MST")

	// Magic date is 2006-01-02T15:04:05 MST"
	fw := NewFileCDRWriter(cdrDirectoryName, "cdr_2006-01-02.txt", lw, 1000)

	fw.WriteRadiusCDR(&rp)

	fw.Close()

	// Open the single file in the path
	fileEntries, err := os.ReadDir(cdrDirectoryName)
	if err != nil {
		t.Fatal("could not list the cdr directory")
	}
	if len(fileEntries) == 0 {
		t.Fatal("cdr directory is empty")
	}

	cdrbytes, err := os.ReadFile(cdrDirectoryName + "/" + fileEntries[0].Name())
	if err != nil {
		t.Fatalf("error reading cdr file %s", err)
	}
	if !strings.Contains(string(cdrbytes), "0102030405060708090a0b") {
		t.Fatal("bad cdr contents")
	}
}

func TestFileWriterRotation(t *testing.T) {

	// For being able to execute a single test
	os.RemoveAll(cdrDirectoryName)

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, "2006-01-02T15:04:05 MST", "2006-01-02T15:04:05 MST")

	// Magic date is 2006-01-02T15:04:05 MST"
	// Rotate in 2 seconds
	fw := NewFileCDRWriter(cdrDirectoryName, "cdr_2006-01-02T15-04-05.txt", lw, 2)

	fw.WriteRadiusCDR(&rp)
	time.Sleep(2100 * time.Millisecond)
	fw.WriteRadiusCDR(&rp)

	fw.Close()

	fileEntries, err := os.ReadDir(cdrDirectoryName)
	if err != nil {
		t.Fatal("could not list the cdr directory")
	}
	if len(fileEntries) != 2 {
		t.Fatal("there should be two files")
	}

	// Check the contents of one of the files
	cdrbytes, err := os.ReadFile(cdrDirectoryName + "/" + fileEntries[1].Name())
	if err != nil {
		t.Fatalf("error reading cdr file %s", err)
	}
	if !strings.Contains(string(cdrbytes), "0102030405060708090a0b") {
		t.Fatal("bad cdr contents")
	}
}

func TestElasticWriter(t *testing.T) {
	var conf ElasticWriterConf
	if err := json.Unmarshal([]byte(jElasticConfig), &conf); err != nil {
		t.Fatalf("bad ElasticWriterConf format: %s", err)
	}
	ew := NewElasticWriter(conf)

	ecdrw := NewElasticCDRWriter("http://elasticdatabase:9200/_doc/_bulk?filter_path=took,errors", "", "", ew, 1 /* Timeout */, 2 /* GlitchSeconds */)

	rp := buildSimpleRadiusPacket(t)
	ecdrw.WriteRadiusCDR(&rp)
	time.Sleep(1 * time.Second)
	//ecdrw.Close()
}

// Helper function
func buildSimpleRadiusPacket(t *testing.T) core.RadiusPacket {
	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-StringAttribute": "anotherStringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1"},
			{"Igor-TimeAttribute": "1986-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-TaggedStringAttribute": "myString:1"},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name": "MyUserName"},
			{"Acct-Input-Octets": 1000},
			{"Acct-Input-Gigawords": 1},
			{"Acct-Session-Time": 3600},
			{"Acct-Delay-Time": 2},
			{"Acct-Session-Id": "session-1"},
			{"NAS-IP-Address": "127.0.0.1"}
		]
	}`

	// Read JSON to Radius Packet
	rp := core.RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	return rp
}
