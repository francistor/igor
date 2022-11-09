package instrumentation

import (
	"fmt"
	"testing"

	"github.com/francistor/igor/config"

	"go.uber.org/zap/zapcore"
)

// Initializer of the test suite.
func TestWideLog(t *testing.T) {

	fakeHandler()
	logger := config.GetLogger()
	logger.Infof("%s %d", "EndingHandler", 0)
	logger.Info(fmt.Sprintf("%s %d", "EndingHandler", 0))

}

func fakeHandler() {
	logLines := make(LogLines, 0)

	defer func(lines []LogLine) {
		logLines.WriteWLog()
	}(logLines)

	logLines.WLogEntry(zapcore.InfoLevel, "%s", "--- StartingHandler")
	logLines.WLogEntry(zapcore.InfoLevel, "%s %d", "EndingHandler", 0)

}
