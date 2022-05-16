package diamserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/instrumentation"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {

	// Initialize the Config Objects
	config.InitConfigurationInstance("resources/searchRules.json", "testServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownClient")
	config.InitConfigurationInstance("resources/searchRules.json", "testClientUnknownServer")
	config.InitConfigurationInstance("resources/searchRules.json", "testServerBadOriginNetwork")
	config.InitConfigurationInstance("resources/searchRules.json", "testSuperServer")

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestBasicSetup(t *testing.T) {
	NewDiameterPeerManager("testSuperServer")
	time.Sleep(150 * time.Millisecond)
	NewDiameterPeerManager("testServer")
	time.Sleep(150 * time.Millisecond)
	NewDiameterPeerManager("testClient")

	time.Sleep(1 * time.Second)

	j, _ := json.Marshal(instrumentation.MS.PeersTableQuery())
	fmt.Println(prettyPrintJSON(j))

	peerTables := instrumentation.MS.PeersTableQuery()

	clientTable := peerTables["testClient"]
	clientPeerServer := findPeer("server.igor", clientTable)
	if clientPeerServer.IsEngaged != true {
		t.Error("server.igor not engaged in client peers table")
	}
	clientPeerBadServer := findPeer("unreachableserver.igor", clientTable)
	if clientPeerBadServer.IsEngaged != false {
		t.Error("unreachableserver.igor engaged in client peers table")
	}

	serverTable := peerTables["testServer"]
	serverPeerClient := findPeer("client.igor", serverTable)
	if serverPeerClient.IsEngaged != true {
		t.Error("server.igor not engaged in server peers table")

	}
	serverPeerSuperServer := findPeer("superserver.igor", serverTable)
	if serverPeerSuperServer.IsEngaged != true {
		t.Error("badserver.igor engaged in server peers table")
	}

	superserverTable := peerTables["testSuperServer"]
	superserverPeerServer := findPeer("server.igor", superserverTable)
	if superserverPeerServer.IsEngaged != true {
		t.Error("server.igor not engaged in superserverserver peers table")
	}

}

// Helper to navigate through peers
func findPeer(diameterHost string, table instrumentation.DiameterPeersTable) instrumentation.DiameterPeersTableEntry {
	for _, tableEntry := range table {
		if tableEntry.DiameterHost == diameterHost {
			return tableEntry
		}
	}

	return instrumentation.DiameterPeersTableEntry{}

}

func prettyPrintJSON(j []byte) string {
	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, j, "", "    "); err != nil {
		return "<bad json>"
	}

	return jBytes.String()
}
