package core

import (
	"net/http"
	"os"
	"testing"
)

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir(os.Getenv("IGOR_BASE") + "resources"))
	http.Handle("/", fileHandler)
	if err := http.ListenAndServe(":8100", nil); err != nil {
		panic("could not start http server")
	}
}

func TestMain(m *testing.M) {

	// Start the server for configuration
	go httpServer()

	// Initialize the Config Objects
	bootFile := "resources/searchRules.json"
	instanceName := "testConfig"

	InitPolicyConfigInstance(bootFile, instanceName, true)
	InitHttpHandlerConfigInstance(bootFile, instanceName, false)

	os.Exit(m.Run())
}
