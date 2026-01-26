package logging

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger is the global logger instance
var Logger *logrus.Logger

func init() {
	Logger = logrus.New()
	// Default to stderr - critical for stdio mode where stdout is MCP protocol
	Logger.SetOutput(os.Stderr)
	Logger.SetLevel(logrus.InfoLevel)
	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}

// Init initializes the logger with the given level and format
func Init(level, format string) error {
	// Parse and set log level
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	Logger.SetLevel(lvl)

	// Set formatter
	switch format {
	case "json":
		Logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		Logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	}

	return nil
}

// Debug logs a debug message
func Debug(args ...any) {
	Logger.Debug(args...)
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...any) {
	Logger.Debugf(format, args...)
}

// Info logs an info message
func Info(args ...any) {
	Logger.Info(args...)
}

// Infof logs a formatted info message
func Infof(format string, args ...any) {
	Logger.Infof(format, args...)
}

// Warn logs a warning message
func Warn(args ...any) {
	Logger.Warn(args...)
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...any) {
	Logger.Warnf(format, args...)
}

// Error logs an error message
func Error(args ...any) {
	Logger.Error(args...)
}

// Errorf logs a formatted error message
func Errorf(format string, args ...any) {
	Logger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits
func Fatal(args ...any) {
	Logger.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(format string, args ...any) {
	Logger.Fatalf(format, args...)
}

// WithField returns a log entry with a field
func WithField(key string, value any) *logrus.Entry {
	return Logger.WithField(key, value)
}

// WithFields returns a log entry with fields
func WithFields(fields logrus.Fields) *logrus.Entry {
	return Logger.WithFields(fields)
}
