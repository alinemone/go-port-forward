// Package logger provides structured logging with rotation support.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level represents the log level.
type Level int

const (
	// LevelDebug for detailed debugging information
	LevelDebug Level = iota
	// LevelInfo for general informational messages
	LevelInfo
	// LevelWarn for warning messages
	LevelWarn
	// LevelError for error messages
	LevelError
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging capabilities.
type Logger struct {
	logger *log.Logger
	level  Level
	file   io.WriteCloser
}

// New creates a new logger instance with rotation support.
func New(maxSizeMB, maxBackups int, level Level) (*Logger, error) {
	logPath := getLogPath()

	// Ensure logs directory exists
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Setup lumberjack for log rotation
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     28, // days
		Compress:   true,
	}

	// Create logger
	logger := log.New(lumberjackLogger, "", 0)

	return &Logger{
		logger: logger,
		level:  level,
		file:   lumberjackLogger,
	}, nil
}

// Close closes the logger.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// ServiceEvent logs a service-related event.
func (l *Logger) ServiceEvent(serviceName, event string, args ...interface{}) {
	message := fmt.Sprintf(event, args...)
	l.Info("[%s] %s", serviceName, message)
}

// ServiceError logs a service-related error.
func (l *Logger) ServiceError(serviceName, errorMsg string, args ...interface{}) {
	message := fmt.Sprintf(errorMsg, args...)
	l.Error("[%s] %s", serviceName, message)
}

// log is the internal logging method.
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, level.String(), message)

	l.logger.Println(logLine)
}

func getLogPath() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("logs", "pf.log")
	}
	exeDir := filepath.Dir(exe)
	return filepath.Join(exeDir, "logs", "pf.log")
}
