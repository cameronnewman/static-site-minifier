package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestNewInfoLevel(t *testing.T) {
	log, err := New(false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if log.Core().Enabled(zapcore.DebugLevel) {
		t.Error("info logger should not enable debug level")
	}
	if !log.Core().Enabled(zapcore.InfoLevel) {
		t.Error("info logger should enable info level")
	}
}

func TestNewDebugLevel(t *testing.T) {
	log, err := New(true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !log.Core().Enabled(zapcore.DebugLevel) {
		t.Error("debug logger should enable debug level")
	}
}
