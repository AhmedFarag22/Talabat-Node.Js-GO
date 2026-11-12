// Package logger wraps log/slog with helpers to attach correlation/user IDs
// from context. Equivalent to lib/logger/logger.ts in the Node services.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/quickbite/analytics-service/lib/appcontext"
)

// New returns a JSON slog.Logger configured to the given level. Use "debug",
// "info", "warn", "error" — anything else falls back to info.
func New(level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

func parseLevel(s string) slog.Leveler {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// FromContext returns a logger pre-attached with correlation_id and (when
// present) user_id from the request context. Pulled in by handlers and the
// consumer at the start of work.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	args := []any{}
	if cid := appcontext.CorrelationID(ctx); cid != "" {
		args = append(args, slog.String("correlation_id", cid))
	}
	if claims := appcontext.Claims(ctx); claims != nil {
		args = append(args, slog.String("user_id", claims.UserID))
	}
	if len(args) == 0 {
		return base
	}
	return base.With(args...)
}
