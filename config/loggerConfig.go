package config

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// Must be initialized with a call to SetupLogger
var ilogger *zap.SugaredLogger

// https://pkg.go.dev/go.uber.org/zap
// Returns a configured instance of zap logger
func initLogger(cm *ConfigurationManager) {

	defaultLogConfig := `{
		"level": "debug",
		"development": true,
		"encoding": "console",
		"outputPaths": ["stdout"],
		"errorOutputPaths": ["stderr"],
		"disableCaller": false,
		"disableStackTrace": false,
		"encoderConfig": {
			"messageKey": "message",
			"levelKey": "level",
			"levelEncoder": "lowercase",
			"callerKey": "caller",
			"callerEncoder": "",
			"timeKey": "ts",
			"timeEncoder": "ISO8601"
			}
		}`

	// Retrieve the log configuration
	jConfig, err := cm.GetConfigObjectAsText("log.json", false)
	if err != nil {
		fmt.Println("using default logging configuration")
		jConfig = []byte(defaultLogConfig)
	}

	var cfg zap.Config
	if err := json.Unmarshal([]byte(jConfig), &cfg); err != nil {
		panic(err)
	}

	logger, logError := cfg.Build()
	if logError != nil {
		panic(logError)
	}

	ilogger = logger.Sugar()
}

// Used globally to get access to the logger
func GetLogger() *zap.SugaredLogger {
	return ilogger
}
