// Package logger provides a preconfigured zap console logger.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New returns a console logger writing to stdout, at debug level when
// debug is true and info level otherwise.
func New(debug bool) (*zap.Logger, error) {
	logLevel := zap.NewAtomicLevelAt(zap.InfoLevel)

	if debug {
		logLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logConfig := zap.Config{
		Encoding:          "console",
		Level:             logLevel,
		DisableCaller:     true,
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
		EncoderConfig:     encoderConfig(),
	}

	return logConfig.Build()
}

func encoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        zapcore.OmitKey,
		LevelKey:       zapcore.OmitKey,
		NameKey:        "N",
		CallerKey:      "C",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}
