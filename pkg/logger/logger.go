// Package logger provides a thin constructor over log/slog so every service
// emits structured logs with a consistent shape. It deliberately stays minimal:
// services obtain a *slog.Logger and use the standard slog API directly.
//
// Conventions:
//   - JSON output in shared environments; text is available for local dev.
//   - Correlation IDs and the principal's user ID are attached as attributes by
//     middleware, never logged ad hoc.
//   - Never log secrets or PII (see the security rules).
package logger

import (
	"io"
	"log/slog"
	"os"
)

// Format selects the output encoding.
type Format string

const (
	// FormatJSON is the default for shared/production environments.
	FormatJSON Format = "json"
	// FormatText is human-friendly for local development.
	FormatText Format = "text"
)

// Options configures a logger. The zero value yields a sane production default
// (JSON, Info level, stdout).
type Options struct {
	Format Format
	Level  slog.Level
	Writer io.Writer
}

// New builds a *slog.Logger from opts, filling defaults for any zero field.
func New(opts Options) *slog.Logger {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	handlerOpts := &slog.HandlerOptions{Level: opts.Level}

	var handler slog.Handler
	switch opts.Format {
	case FormatText:
		handler = slog.NewTextHandler(opts.Writer, handlerOpts)
	default:
		handler = slog.NewJSONHandler(opts.Writer, handlerOpts)
	}
	return slog.New(handler)
}

// Default returns a production-default logger (JSON, Info, stdout).
func Default() *slog.Logger {
	return New(Options{Format: FormatJSON, Level: slog.LevelInfo})
}
