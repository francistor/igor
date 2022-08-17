package handlerfunctions

import (
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

	fmt.Printf("%#v\n", entry)
}
