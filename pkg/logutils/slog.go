package logutils

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
)

// SlogHandler returns an slog.Handler that writes through logger, for
// libraries that only accept *slog.Logger. Groups become dotted key prefixes.
func SlogHandler(logger zerolog.Logger) slog.Handler {
	return &slogHandler{logger: logger}
}

type slogHandler struct {
	logger zerolog.Logger
	prefix string
}

func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return zerologLevel(level) >= h.logger.GetLevel()
}

func (h *slogHandler) Handle(ctx context.Context, record slog.Record) error {
	h.logger.WithLevel(zerologLevel(record.Level)).Ctx(ctx).Func(func(event *zerolog.Event) {
		record.Attrs(func(attr slog.Attr) bool {
			addAttr(event, h.prefix, attr)
			return true
		})
	}).Msg(record.Message)
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	ctx := h.logger.With()
	for _, attr := range attrs {
		ctx = addContextAttr(ctx, h.prefix, attr)
	}
	return &slogHandler{logger: ctx.Logger(), prefix: h.prefix}
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &slogHandler{logger: h.logger, prefix: h.prefix + name + "."}
}

func zerologLevel(level slog.Level) zerolog.Level {
	switch {
	case level >= slog.LevelError:
		return zerolog.ErrorLevel
	case level >= slog.LevelWarn:
		return zerolog.WarnLevel
	case level >= slog.LevelInfo:
		return zerolog.InfoLevel
	default:
		return zerolog.DebugLevel
	}
}

func addAttr(event *zerolog.Event, prefix string, attr slog.Attr) {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		for _, member := range value.Group() {
			addAttr(event, groupPrefix(prefix, attr.Key), member)
		}
		return
	}
	if attr.Key == "" {
		return
	}
	event.Interface(prefix+attr.Key, value.Any())
}

func addContextAttr(ctx zerolog.Context, prefix string, attr slog.Attr) zerolog.Context {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		for _, member := range value.Group() {
			ctx = addContextAttr(ctx, groupPrefix(prefix, attr.Key), member)
		}
		return ctx
	}
	if attr.Key == "" {
		return ctx
	}
	return ctx.Interface(prefix+attr.Key, value.Any())
}

// An inline group (empty key) adds its members at the current level, per the
// slog.Handler contract.
func groupPrefix(prefix, key string) string {
	if key == "" {
		return prefix
	}
	return prefix + key + "."
}
