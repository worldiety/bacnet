package log

import (
	"log/slog"
	"os"
)

var Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	AddSource:   false,
	Level:       slog.LevelError,
	ReplaceAttr: nil,
}))

func InitLogger(level slog.Level, addSource bool) {
	Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   addSource,
		Level:       level,
		ReplaceAttr: nil,
	}))
}
