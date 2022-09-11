package config

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// Must be initialized with a call to initLogger, which is done
// during the initialization of a default policyConfigurationManager or
// handlerConfigurationManager
var ilogger *zap.SugaredLogger

// https://pkg.go.dev/go.uber.org/zap
// Returns a configured instance of zap logger
func initLogger(cm *ConfigurationManager) {

	defaultLogConfig := `{
		"level": "info",
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

	// Parse the JSON
	var cfg zap.Config
	if err := json.Unmarshal(jConfig, &cfg); err != nil {
		panic("bad log configuration " + err.Error())
	}

	// Build a logger with the specified configuration
	logger, err := cfg.Build()
	if err != nil {
		panic("bad log configuration " + err.Error())
	}

	ilogger = logger.Sugar()
}

// Used globally to get access to the logger
func GetLogger() *zap.SugaredLogger {
	return ilogger
}
