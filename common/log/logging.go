package log

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger = slog.New(slog.NewTextHandler(nil, nil))

func InitLogger(level slog.Level, addSource bool) {
	Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   addSource,
		Level:       level,
		ReplaceAttr: nil,
	}))
}
