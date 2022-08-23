package cdrwriter

import (
	"encoding/json"
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
var cdrDirectoryName = os.Getenv("IGOR_BASE") + "cdr"

// Initializer of the test suite.
func TestMain(m *testing.M) {
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, true)

	// Execute the tests
	exitCode := m.Run()

	// Clean cdr files
	//os.RemoveAll(cdrDirectoryName)

	os.Exit(exitCode)
}

func TestLivingstoneFormat(t *testing.T) {

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, time.RFC3339, time.RFC3339)
	cdrString := lw.GetCDRString(&rp)
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
		"Igor-SaltedOctetsAttribute"},
		";", ",", time.RFC3339, true)
	cdrString := csvw.GetCDRString(&rp)
	if strings.Contains(cdrString, "MyUserName") {
		t.Fatalf("Written CDR contains filtered attribute User-Name")
	}
	if !strings.Contains(cdrString, "\"00aabbccddeeff11\"") {
		t.Fatalf("missing attribute in written string")
	}
	if !strings.Contains(cdrString, ";;") {
		t.Fatalf("pattern for non existing attribute not found")
	}
}

func TestFileWriter(t *testing.T) {

	// For being able to execute a single test
	os.RemoveAll(cdrDirectoryName)

	// Read JSON to Radius Packet
	rp := buildSimpleRadiusPacket(t)

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, time.RFC3339, time.RFC3339)

	// Magic date is 2006-01-02T15:04:05 UTC"
	fw := NewFileCDRWriter(cdrDirectoryName, "cdr_2006-01-02", lw, 1000)

	fw.WriteCDR(&rp)

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

	lw := NewLivingstoneWriter(nil, []string{"User-Name"}, time.RFC3339, time.RFC3339)

	// Magic date is 2006-01-02T15:04:05 UTC"
	// Rotate in 2 seconds
	fw := NewFileCDRWriter(cdrDirectoryName, "cdr_2006-01-02T15-04-05", lw, 2)

	fw.WriteCDR(&rp)
	time.Sleep(2100 * time.Millisecond)
	fw.WriteCDR(&rp)

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

// Helper function
func buildSimpleRadiusPacket(t *testing.T) radiuscodec.RadiusPacket {
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

	return rp
}
