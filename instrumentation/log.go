package instrumentation

import (
	"fmt"
	"sync"

	"github.com/francistor/igor/config"

	"go.uber.org/zap/zapcore"
)

/*
Utilities for Wide log implementation

The function using this must declare a var of type WideLogger and invoke Log on it per line to
be written. The write will write all entries, and is typically invoked in a defer function

	wideLogger := instrumentation.NewWideLogger()

	defer func(lines *instrumentation.WideLogger) {
		wideLogger.Write()
	}(wideLogger)
*/

// Represents a log entry to be written on function exit
type LogLine struct {
	level zapcore.Level
	log   string
}

// The set of log entries to be written on function exit
type WideLogger struct {
	lines []LogLine
	wchan chan LogLine
	wg    sync.WaitGroup
}

// Creates a Wide logger object
func NewWideLogger() *WideLogger {
	r := WideLogger{
		lines: make([]LogLine, 1),
		wchan: make(chan LogLine),
	}

	// Reader and appender until the channel is closed
	go func() {
		for l := range r.wchan {
			r.lines = append(r.lines, l)
		}

		logger := config.GetLogger()
		for i := range r.lines {
			if config.IsLevelEnabled(r.lines[i].level) {
				switch r.lines[i].level {
				case zapcore.DebugLevel:
					logger.Debug(r.lines[i].log)
				case zapcore.InfoLevel:
					logger.Info(r.lines[i].log)
				case zapcore.WarnLevel:
					logger.Warn(r.lines[i].log)
				case zapcore.ErrorLevel:
					logger.Error(r.lines[i].log)
				}
			}
		}

	}()

	return &r
}

// Adds a log entry to the slize of entries to be written
func (l *WideLogger) Log(level zapcore.Level, format string, args ...interface{}) {
	if config.IsLevelEnabled(level) {
		line := fmt.Sprintf(format, args...)
		l.wchan <- LogLine{level: level, log: line}
	}
}

// To be invoked on each new goroutine in the same way as a WaitGroup
func (l *WideLogger) Add() {
	l.wg.Add(1)
}

// To be invoked on each new goroutine in the same way as a WaitGroup
func (l *WideLogger) Done() {
	l.wg.Done()
}

// Writes the log lines
func (l *WideLogger) Write() {
	// Waits for routines to finish
	l.wg.Wait()

	// Close the channel. The log will be written
	close(l.wchan)
}
