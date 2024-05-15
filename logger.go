package main

import (
	"log/slog"
	"os"
)

func makeLogger() *slog.Logger {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	return logger
}
