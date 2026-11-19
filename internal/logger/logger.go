// Package logger configures structured application logging.
package logger

import (
	"errors"
	"io"
	"log/slog"

	"github.com/akhmed9505/comment-tree/internal/config"
)

// New returns a logger configured to write to output without changing the default logger.
func New(cfg config.Logging, output io.Writer) (*slog.Logger, error) {
	if output == nil {
		return nil, errors.New("logger output is required")
	}
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, errors.New("logger level must be debug, info, warn, or error")
	}
	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, errors.New("logger format must be json or text")
	}
	return slog.New(handler), nil
}
