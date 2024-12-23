package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

// LogstashHook is a logrus hook for publishing logs to logstash
type LogstashHook struct {
	Endpoint string
}

// NewLogstashHook creates a new instance of LogstashHook
func NewLogstashHook(endpoint string) *LogstashHook {
	return &LogstashHook{
		Endpoint: endpoint,
	}
}

// Fire() publishes the given entry to Logstash
func (hook *LogstashHook) Fire(entry *logrus.Entry) error {
	// Convert the log entry to JSON
	data, err := json.Marshal(entry.Data)
	if err != nil {
		return err
	}

	// Create a new HTTP request to send the log entry to Logstash
	req, err := http.NewRequest("POST", hook.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send log entry to Logstash, status code: %d", resp.StatusCode)
	}

	return nil
}

// Levels includes all log levels for publishing
func (hook *LogstashHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// LogstashLogger is a logr.Logger wrapper adding logrus Logstash hook
type LogstashLogger struct {
	logr.Logger
	logrusLogger *logrus.Logger
}

// NewLogstashLogger creates a new instance of LogstashLogger
func NewLogstashLogger(logrLogger logr.Logger, logstashEndpoint string) *LogstashLogger {
	logrusLogger := logrus.New()
	if logstashEndpoint != "" {
		logstashHook := NewLogstashHook(logstashEndpoint)
		logrusLogger.Hooks.Add(logstashHook)
	}

	return &LogstashLogger{
		Logger:       logrLogger,
		logrusLogger: logrusLogger,
	}
}

// Info publishes an info-level message
func (l *LogstashLogger) Info(msg string, keysAndValues ...interface{}) {
	l.Logger.Info(msg, keysAndValues...)
	l.logrusLogger.WithFields(convertToLogrusFields(keysAndValues...)).Info(msg)
}

// Error publishes an error-level message
func (l *LogstashLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.Logger.Error(err, msg, keysAndValues...)
	l.logrusLogger.WithFields(convertToLogrusFields(keysAndValues...)).WithError(err).Error(msg)
}

// V sets the given verbosity level
func (l *LogstashLogger) V(level int) *LogstashLogger {
	return &LogstashLogger{
		Logger:       l.Logger.V(level),
		logrusLogger: l.logrusLogger,
	}
}

// WithValues sets the given values
func (l *LogstashLogger) WithValues(keysAndValues ...interface{}) *LogstashLogger {
	return &LogstashLogger{
		Logger:       l.Logger.WithValues(keysAndValues...),
		logrusLogger: l.logrusLogger,
	}
}

// WithValues sets the given name
func (l *LogstashLogger) WithName(name string) *LogstashLogger {
	return &LogstashLogger{
		Logger:       l.Logger.WithName(name),
		logrusLogger: l.logrusLogger,
	}
}

// convertToLogrusFields puts the given keys and values into a logrus.Fields instance
func convertToLogrusFields(keysAndValues ...interface{}) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fields[keysAndValues[i].(string)] = keysAndValues[i+1]
		}
	}
	return fields
}
