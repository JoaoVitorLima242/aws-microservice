// Package logging configures a JSON slog logger that writes one line per
// event to stdout and automatically attaches the request correlation ID
// stored in the context.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

type requestIDKey struct{}

// WithRequestID returns a copy of ctx carrying the correlation ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the correlation ID stored in ctx, or "" if there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// New builds the service logger. The returned bool is false when level was
// not recognised (the logger then falls back to info).
func New(service, version, level string) (*slog.Logger, bool) {
	lvl, ok := parseLevel(level)
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.TimeKey:
				return slog.String("timestamp", a.Value.Time().UTC().Format(time.RFC3339Nano))
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	})
	logger := slog.New(contextHandler{h}).With(
		slog.String("service", service),
		slog.String("version", version),
	)
	return logger, ok
}

func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "", "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// contextHandler adds the requestId attribute when the context carries one.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("requestId", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
