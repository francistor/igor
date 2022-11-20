package instrumentation

import (
	"fmt"

	"github.com/francistor/igor/config"

	"go.uber.org/zap/zapcore"
)

/*
Utilities for Wide log implementation

The function using this must declare a var of type LogLines and invoke WLogEntry on it per line to
be written. The WriteWLog will write all entries, and is typically invoked in a defer function

	defer func(lines []instrumentation.LogLine) {
		logLines.WriteWLog()
	}(logLines)
*/

// Represents a log entry to be written on function exit
type LogLine struct {
	level zapcore.Level
	log   string
}

// The set of log entries to be written on function exit
type LogLines struct {
	Lines []LogLine
	//wg    sync.WaitGroup
}

func NewLogLines() *LogLines {
	return &LogLines{
		Lines: make([]LogLine, 1),
	}
}

// Adds a log entry to the slize of entries to be written
func (l *LogLines) WLogEntry(level zapcore.Level, format string, args ...interface{}) {
	if config.IsLevelEnabled(level) {
		line := fmt.Sprintf(format, args...)
		l.Lines = append(l.Lines, LogLine{level: level, log: line})
	}
}

// Writes the log lines
func (l *LogLines) WriteWLog() {
	logger := config.GetLogger()
	for i := range l.Lines {
		if config.IsLevelEnabled(l.Lines[i].level) {
			switch l.Lines[i].level {
			case zapcore.DebugLevel:
				logger.Debug(l.Lines[i].log)
			case zapcore.InfoLevel:
				logger.Info(l.Lines[i].log)
			case zapcore.WarnLevel:
				logger.Warn(l.Lines[i].log)
			case zapcore.ErrorLevel:
				logger.Error(l.Lines[i].log)
			}
		}
	}
}
