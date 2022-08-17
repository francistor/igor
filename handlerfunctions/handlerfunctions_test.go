package handlerfunctions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/config"
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

	entry, err := NewRadiusUserFileEntry("key1", "radiusUserFile.json", config.GetPolicyConfigInstance("testConfig"))
	if err != nil {
		t.Fatal(err)
	}

	jEntry, err := json.Marshal(entry)

	fmt.Println(PrettyPrintJSON(jEntry))
}

// Helper to show JSON to humans
func PrettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
