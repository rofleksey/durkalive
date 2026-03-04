package conversation

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// WithLogger returns a context that carries the given logger.
func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, log)
}

// LoggerFromContext returns the logger from ctx, or slog.Default() if none.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}
