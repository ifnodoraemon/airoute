package telemetry

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var Logger *slog.Logger

func init() {
	InitLogger("info")
}

// InitLogger initializes the global structured slog logger.
func InitLogger(levelStr string) {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	broadcastHandler := NewBroadcastHandler(jsonHandler, GlobalRingBuffer, level)
	Logger = slog.New(broadcastHandler)
}

type scopedLoggerEntry struct {
	base   *slog.Logger
	logger *slog.Logger
}

// scopedLoggers caches the per-category loggers so a hot call site does not
// rebuild the handler chain for every record. Entries are rebound whenever
// InitLogger replaces the base logger.
var scopedLoggers sync.Map // category -> scopedLoggerEntry

// ScopedLogger returns the global logger tagged with a structured category.
// Subsystems use it instead of relying on BroadcastHandler's message-text
// heuristic, which guessed the category from words like "redis" or "channel".
func ScopedLogger(category string) *slog.Logger {
	base := Logger
	if cached, ok := scopedLoggers.Load(category); ok {
		entry := cached.(scopedLoggerEntry)
		if entry.base == base {
			return entry.logger
		}
	}
	logger := base.With("category", category)
	scopedLoggers.Store(category, scopedLoggerEntry{base: base, logger: logger})
	return logger
}
