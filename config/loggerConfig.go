package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Two types of loggers are used
// The Core logger (or Igor logger) is general to Igor, initialized in the initLogger
// method. Only a single instance exists
// The Handler loggers are created on each handler, using NewHandlerLogger

type LogConfig struct {
	CoreLogConfig    json.RawMessage
	HandlerLogConfig json.RawMessage
}

type HandlerLogger struct {
	L *zap.SugaredLogger
	b bytes.Buffer
}

const (
	LEVEL_DEBUG = -1
	LEVEL_INFO  = 0
	LEVEL_WARN  = 1
	LEVEL_ERROR = 2
)

// Must be initialized with a call to initLogger, which is done
// during the initialization of a default policyConfigurationManager or
// handlerConfigurationManager
var ilogger *zap.SugaredLogger

// The configured logLevel for the core logger
var ilogLevel zapcore.Level

// The configuration for the handler loggers
var handlerCfg zap.Config

// https://pkg.go.dev/go.uber.org/zap
// Returns a configured instance of zap logger
func initLogger(cm *ConfigurationManager) {

	defaultLogConfig := `{
		"coreLogConfig":{
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
		},
		"handlerLogConfig": {
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
		}
	}`

	// Retrieve the log configuration
	jConfig, err := cm.GetBytesConfigObject("log.json")
	if err != nil {
		fmt.Println("using default logging configuration: " + err.Error())
		jConfig = []byte(defaultLogConfig)
	}

	// Parse complete JSON
	var rawConfig LogConfig
	if err := json.Unmarshal(jConfig, &rawConfig); err != nil {
		panic("bad log configuration " + err.Error())
	}

	// Global Logger Configuration

	// Parse the core logger configuration
	var coreCfg zap.Config
	if err := json.Unmarshal(rawConfig.CoreLogConfig, &coreCfg); err != nil {
		panic("bad core log configuration " + err.Error())
	}

	// Build a logger with the specified configuration
	logger, err := coreCfg.Build()
	if err != nil {
		panic("bad core log configuration " + err.Error())
	}

	ilogLevel = coreCfg.Level.Level()

	ilogger = logger.Sugar()

	// Handler Loggers Configuration

	if err := json.Unmarshal(rawConfig.HandlerLogConfig, &handlerCfg); err != nil {
		panic("bad handler log configuration " + err.Error())
	}
}

// Creates a new HandlerLogger
func NewHandlerLogger() *HandlerLogger {

	handlerLogger := HandlerLogger{}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(handlerCfg.EncoderConfig),
		zapcore.AddSync(&handlerLogger.b),
		handlerCfg.Level,
	)

	handlerLogger.L = zap.New(core).Sugar()

	return &handlerLogger
}

func (h *HandlerLogger) String() string {
	// Probably unecessary
	h.L.Sync()

	return h.b.String()
}

// Writes the handler log using the highest compatible log level
func (h *HandlerLogger) WriteLog() {

	text := h.String()
	if text != "" {
		if ilogLevel.Enabled(zapcore.DebugLevel) {
			ilogger.Debugln(text)
		} else if ilogLevel.Enabled(zapcore.InfoLevel) {
			ilogger.Infoln(text)
		} else if ilogLevel.Enabled(zapcore.WarnLevel) {
			ilogger.Warnln(text)
		} else if ilogLevel.Enabled(zapcore.ErrorLevel) {
			ilogger.Errorln(text)
		}
	}
}

// Check whether the specified log level is enabled
func (h *HandlerLogger) IsLevelEnabled(level int) bool {
	switch level {
	case LEVEL_ERROR:
		return handlerCfg.Level.Enabled(zapcore.ErrorLevel)
	case LEVEL_WARN:
		return handlerCfg.Level.Enabled(zapcore.WarnLevel)
	case LEVEL_INFO:
		return handlerCfg.Level.Enabled(zapcore.InfoLevel)
	case LEVEL_DEBUG:
		return handlerCfg.Level.Enabled(zapcore.DebugLevel)
	}

	return true
}

// Used globally to get access to the core logger
func GetLogger() *zap.SugaredLogger {
	return ilogger
}

func IsDebugEnabled() bool {
	return ilogLevel.Enabled(zapcore.DebugLevel)
}

func IsInfoEnabled() bool {
	return ilogLevel.Enabled(zapcore.InfoLevel)
}

func IsWarnEnabled() bool {
	return ilogLevel.Enabled(zapcore.WarnLevel)
}

func IsErrorEnabled() bool {
	return ilogLevel.Enabled(zapcore.ErrorLevel)
}

func IsLevelEnabled(level zapcore.Level) bool {
	return ilogLevel.Enabled(level)
}
