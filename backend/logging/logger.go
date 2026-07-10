package logging

import (
	"io"
	"log/slog"
	"os"
)

func Init(logLevel string) {
	level := slog.LevelInfo
	if logLevel == "debug" {
		level = slog.LevelDebug
	}

	var w io.Writer = os.Stdout

	Logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(Logger)
}
