// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/trace"
	_zap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// FromContextOrDiscard retrieves the logger from the context and adds trace context values to it
// if available.
// If no logger is found in the context, it returns a no-op logger.
func FromContextOrDiscard(ctx context.Context) logr.Logger {
	logger := logr.FromContextOrDiscard(ctx)
	for key, value := range carrierFromContext(ctx) {
		logger = logger.WithValues(key, value)
	}
	return logger
}

type ETOSLogger struct {
	logger   logr.Logger
	opts     zap.Options
	logcores []zapcore.Core
}

// New creates a new ETOSLogger with the provided zap.Options. It adds default values to the
// options if they are not already set.
func New(opts zap.Options) *ETOSLogger {
	return &ETOSLogger{opts: addDefaults(opts)}
}

// WithOtel adds an OpenTelemetry core to the logger if the provided tracer has a LoggerProvider.
func (l *ETOSLogger) WithOtel(tracer *opentelemetry.ETOSTracer) *ETOSLogger {
	if tracer.LoggerProvider == nil {
		return l
	}
	l.logcores = append(l.logcores,
		otelzap.NewCore(tracer.Name, otelzap.WithLoggerProvider(tracer.LoggerProvider)),
	)
	return l
}

// WithConsole adds a console core to the logger that writes to the destination writer specified in the options.
func (l *ETOSLogger) WithConsole() *ETOSLogger {
	l.logcores = append(l.logcores, zapcore.NewCore(
		l.opts.Encoder,
		zapcore.AddSync(l.opts.DestWriter),
		l.opts.Level,
	))
	return l
}

// WithUserLog adds a user log core to the logger that writes to the provided messaging.Publisher.
// If the publisher is nil, it does not add the user log core.
func (l *ETOSLogger) WithUserLog(publisher messaging.Publisher) *ETOSLogger {
	if publisher == nil {
		return l
	}
	l.logcores = append(l.logcores, zapcore.NewCore(
		l.opts.Encoder,
		zapcore.AddSync(newUserLogWriter(publisher)),
		_zap.DebugLevel,
	))
	return l
}

// Start initializes logcores and creates a new zap.Logger with the combined cores.
// It then adds the logger to the context and returns it.
func (l *ETOSLogger) Start(ctx context.Context) context.Context {
	l.opts.ZapOpts = append(l.opts.ZapOpts, _zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(l.logcores...)
	}))

	l.logger = zap.New(zap.UseFlagOptions(&l.opts))
	return logr.NewContext(ctx, l.logger)
}

// carrierFromContext extracts the trace context from the provided context and returns it as a
// map[string]string.
func carrierFromContext(ctx context.Context) map[string]string {
	spanCtx := trace.SpanContextFromContext(ctx)
	carrier := make(map[string]string)
	if spanCtx.HasTraceID() {
		carrier["trace_id"] = spanCtx.TraceID().String()
	}
	if spanCtx.HasSpanID() {
		carrier["span_id"] = spanCtx.SpanID().String()
	}
	return carrier
}

// addDefaults adds default values to the provided zap.Options if they are not already set.
// Zap does have an addDefaults function but it is not exported, so we need to implement our own
// version here.
func addDefaults(opts zap.Options) zap.Options {
	if opts.DestWriter == nil {
		opts.DestWriter = os.Stdout
	}

	if opts.NewEncoder == nil {
		opts.NewEncoder = func(opts ...zap.EncoderConfigOption) zapcore.Encoder {
			encoderConfig := zapcore.EncoderConfig{
				MessageKey:     "message",
				LevelKey:       "levelname",
				TimeKey:        "@timestamp",
				NameKey:        "name",
				CallerKey:      "caller",
				FunctionKey:    zapcore.OmitKey,
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.EpochTimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			}
			for _, opt := range opts {
				opt(&encoderConfig)
			}
			return zapcore.NewJSONEncoder(encoderConfig)
		}
	}
	if opts.Level == nil {
		lvl := _zap.NewAtomicLevelAt(_zap.InfoLevel)
		opts.Level = &lvl
	}
	if opts.StacktraceLevel == nil {
		lvl := _zap.NewAtomicLevelAt(_zap.ErrorLevel)
		opts.StacktraceLevel = &lvl
	}

	if opts.TimeEncoder == nil {
		opts.TimeEncoder = zapcore.TimeEncoderOfLayout("2006-01-02T15:04:05.000Z")
	}
	f := func(ecfg *zapcore.EncoderConfig) {
		ecfg.EncodeTime = opts.TimeEncoder
	}
	// prepend instead of append it in case someone adds a time encoder option in it
	opts.EncoderConfigOptions = append([]zap.EncoderConfigOption{f}, opts.EncoderConfigOptions...)
	if opts.Encoder == nil {
		opts.Encoder = opts.NewEncoder(opts.EncoderConfigOptions...)
	}
	opts.ZapOpts = append(opts.ZapOpts, _zap.AddStacktrace(opts.StacktraceLevel))
	return opts
}

type userLogs struct {
	publisher messaging.Publisher
}

// newUserLogWriter creates a new userLogs instance.
func newUserLogWriter(publisher messaging.Publisher) *userLogs {
	return &userLogs{publisher: publisher}
}

type entry struct {
	Message        string `json:"message"`
	Level          string `json:"levelname"`
	Identifier     string `json:"identifier"`
	DisableUserLog bool   `json:"disableUserLog"`
}

type event struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

// Write implements the io.Writer interface for userLogs. It unmarshals the log entry and
// publishes it to the ETOS messagebus if it contains an identifier and is not disabled for
// user logs.
func (u *userLogs) Write(p []byte) (n int, err error) {
	e := entry{}
	if err := json.Unmarshal(p, &e); err != nil {
		return 0, fmt.Errorf("failed to unmarshal log entry: %w", err)
	}
	if !u.shouldLog(e) {
		return len(p), nil
	}
	message, err := u.formatLog(p)
	if err != nil {
		return 0, fmt.Errorf("failed to format log entry: %w", err)
	}
	routingKey := fmt.Sprintf("%s.message.%s", e.Identifier, e.Level)
	if err := u.publisher.Publish(message, routingKey); err != nil {
		return 0, fmt.Errorf("failed to publish log entry: %w", err)
	}
	return len(p), nil
}

// formatLog formats the log entry by unmarshaling it into a map and then marshaling it back
// into JSON with an "event" field.
func (u *userLogs) formatLog(p []byte) ([]byte, error) {
	data := map[string]any{}
	if err := json.Unmarshal(p, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal log entry into map: %w", err)
	}
	event := event{Event: "message", Data: data}
	return json.Marshal(event)
}

// shouldLog determines whether a log entry should be published to the ETOS messagebus.
// It checks if the entry has an identifier and is not disabled for user logs.
func (u *userLogs) shouldLog(e entry) bool {
	return !e.DisableUserLog && e.Identifier != ""
}
