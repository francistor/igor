package instrumentation

import (
	"fmt"
	"testing"

	"github.com/francistor/igor/config"
)

// Initializer of the test suite.
func TestWideLog(t *testing.T) {

	fakeHandler()
	logger := config.GetLogger()
	logger.Infof("%s %d", "EndingHandler", 0)
	logger.Info(fmt.Sprintf("%s %d", "EndingHandler", 0))

}

func fakeHandler() {
	wideLogger := NewWideLogger()

	defer func(lines *WideLogger) {
		wideLogger.Write()
	}(wideLogger)

	wideLogger.Log(config.LEVEL_INFO, "%s", "--- StartingHandler")
	wideLogger.Log(config.LEVEL_INFO, "%s %d", "EndingHandler", 0)

}
