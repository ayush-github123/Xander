package logger

import (
	"fmt"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Logger struct {
	level Level
}

func New(level string) *Logger {
	logLevel := LevelInfo
	switch level {
	case "debug":
		logLevel = LevelDebug
	case "info":
		logLevel = LevelInfo
	case "warn":
		logLevel = LevelWarn
	case "error":
		logLevel = LevelError
	}

	return &Logger{level: logLevel}
}

func (l *Logger) Debug(msg string, fields ...interface{}) {
	if l.level <= LevelDebug {
		l.logf("DEBUG", msg, fields...)
	}
}

func (l *Logger) Info(msg string, fields ...interface{}) {
	if l.level <= LevelInfo {
		l.logf("INFO", msg, fields...)
	}
}

func (l *Logger) Warn(msg string, fields ...interface{}) {
	if l.level <= LevelWarn {
		l.logf("WARN", msg, fields...)
	}
}

func (l *Logger) Error(msg string, fields ...interface{}) {
	if l.level <= LevelError {
		l.logf("ERROR", msg, fields...)
	}
}

func (l *Logger) logf(level, msg string, fields ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	fieldStr := ""
	if len(fields) > 0 {
		fieldStr = fmt.Sprintf(" %v", fields)
	}
	fmt.Printf("[%s] %s: %s%s\n", timestamp, level, msg, fieldStr)
}
