package sessionserver

import (
	"embed"
	"os"
	"testing"

	"github.com/francistor/igor/core"
)

func TestMain(m *testing.M) {
	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceNameMain := "testSessionMain"
	instanceNameReplica1 := "testSessionReplica1"

	// For sending radius packets to myself
	core.InitPolicyConfigInstance(bootFile, instanceNameMain, nil, embed.FS{}, true)

	// Intialization of Session Servers
	core.InitRadiusSessionServerConfigInstance(bootFile, instanceNameMain, nil, embed.FS{}, false)
	// Intialization of Session Servers
	core.InitRadiusSessionServerConfigInstance(bootFile, instanceNameReplica1, nil, embed.FS{}, false)

	// Execute the tests
	os.Exit(m.Run())
}
