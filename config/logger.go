package config

import (
	"encoding/json"

	"go.uber.org/zap"
)

var IgorLogger *zap.SugaredLogger

func SetupLogger() {
	// Setup logger
	rawJSON := []byte(`{
		"level": "debug",
		"encoding": "json",
		"outputPaths": ["stdout"],
		"errorOutputPaths": ["stderr"],
		"encoderConfig": {
			"messageKey": "message",
			"levelKey": "level",
			"levelEncoder": "lowercase"
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
