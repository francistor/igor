package sessionserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/francistor/igor/core"
)

var bootstrapFile = "../resources/searchRules.json"

func TestServerSunnyDay(t *testing.T) {

	time.Sleep(1 * time.Second)

	core.InitRadiusSessionServerConfigInstance(bootstrapFile, "testSessionMain", nil, true)

	fmt.Println(core.GetRadiusSessionServerConfig())

}
