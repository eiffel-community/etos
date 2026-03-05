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
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/go-logr/logr"
	_zap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// newLogger creates a new logger with the provided options and name.
// It initializes an OpenTelemetry logger and combines it with a Zap logger and a user log core.
// The resulting logger is returned as a logr.Logger.
func newLogger(ctx context.Context, params Parameters, publisher messaging.Publisher) (logr.Logger, error) {
	opts := addDefaults(params.opts)

	cores := []zapcore.Core{zapcore.NewCore(opts.Encoder, zapcore.AddSync(opts.DestWriter), opts.Level)}
	otelCore, err := initOpentelemetryLogger(ctx, params.providerName)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry logger: %v\n", err)
	} else if otelCore != nil {
		cores = append(cores, otelCore)
	}
	var userLogWriter *userLogs
	if publisher != nil {
		userLogWriter = newUserLogWriter(publisher)
		cores = append(cores, zapcore.NewCore(opts.Encoder, zapcore.AddSync(userLogWriter), _zap.DebugLevel))
	}

	// We wrap the core to combine the OpenTelemetry core, the Zap core, and the user log core into
	// a single core that can be used by the logger.
	opts.ZapOpts = append(opts.ZapOpts, _zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(cores...)
	}))
	logger := zap.New(zap.UseFlagOptions(&opts))
	if userLogWriter != nil {
		userLogWriter.setLogger(logger)
	}
	return logger, nil
}

// addDefaults adds default values to the provided zap.Options if they are not already set.
// Zap does have an addDefaults function but it is not exported, so we need to implement our own version here.
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
