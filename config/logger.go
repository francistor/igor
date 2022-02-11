package config

import (
	"encoding/json"

	"go.uber.org/zap"
)

var IgorLogger *zap.SugaredLogger

// https://pkg.go.dev/go.uber.org/zap
func SetupLogger() {
	// Setup logger
	rawJSON := []byte(`{
		"level": "debug",
		"development": true,
		"encoding": "json",
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
		}`)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}

	var logError error
	var logger *zap.Logger
	logger, logError = cfg.Build()
	if logError != nil {
		panic(logError)
	}

	IgorLogger = logger.Sugar()
}
