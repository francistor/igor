package sessionserver

import (
	"os"
	"testing"

	"github.com/francistor/igor/core"
)

func TestMain(m *testing.M) {
	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testConfig"

	core.InitPolicyConfigInstance(bootFile, instanceName, nil, true)

	// Execute the tests
	os.Exit(m.Run())
}
