package sessionserver

import (
	"os"
	"testing"

	"github.com/francistor/igor/core"
)

func TestMain(m *testing.M) {
	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testSessionMain"

	// For sending radius packets to myself
	core.InitPolicyConfigInstance(bootFile, instanceName, nil, true)

	// Intialization of Session Server
	core.InitRadiusSessionServerConfigInstance(bootFile, instanceName, nil, false)

	// Execute the tests
	os.Exit(m.Run())
}
