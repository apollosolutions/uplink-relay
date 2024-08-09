package logger

import (
	"log/slog"
	"os"
)

type keyType string

const LoggerKey keyType = "logger"

// makeLogger creates a new logger instance.
func MakeLogger(enableDebug *bool) *slog.Logger {
	lvl := new(slog.LevelVar)

	if enableDebug == nil {
		lvl.Set(slog.LevelInfo)
	} else if *enableDebug {
		lvl.Set(slog.LevelDebug)
	} else {
		lvl.Set(slog.LevelInfo)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	}))
	slog.SetDefault(logger)

	return logger
}
