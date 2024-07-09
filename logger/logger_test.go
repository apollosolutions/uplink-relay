package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestMakeLogger(t *testing.T) {
	// Test case 1: Enable debug mode
	enableDebug := true
	logger := MakeLogger(&enableDebug)
	if logger == nil {
		t.Error("Expected logger instance, got nil")
	}
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected logger level to be LevelDebug")
	}

	// Test case 2: Disable debug mode
	enableDebug = false
	logger = MakeLogger(&enableDebug)
	if logger == nil {
		t.Error("Expected logger instance, got nil")
	}
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected logger level to be LevelInfo")
	}

	// Test case 3: Passing a nil value
	logger = MakeLogger(nil)
	if logger == nil {
		t.Error("Expected logger instance, got nil")
	}
}
