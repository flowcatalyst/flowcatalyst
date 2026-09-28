package fn

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
)

type logMeta struct {
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// slogHandler implements slog.Handler over host op 1 (log). Failures to
// deliver a log record are swallowed: logging must never fail a handler.
type slogHandler struct {
	prefix string
	attrs  map[string]any
}

func (h *slogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *slogHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(h.attrs)+r.NumAttrs())
	maps.Copy(attrs, h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.prefix != "" {
			key = h.prefix + "." + key
		}
		attrs[key] = a.Value.Any()
		return true
	})
	meta := logMeta{Level: r.Level.String(), Msg: r.Message}
	if len(attrs) > 0 {
		meta.Attrs = attrs
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	_, _, _, _ = currentHost.call(opLog, mb, nil)
	return nil
}

func (h *slogHandler) WithAttrs(as []slog.Attr) slog.Handler {
	next := &slogHandler{prefix: h.prefix, attrs: make(map[string]any, len(h.attrs)+len(as))}
	maps.Copy(next.attrs, h.attrs)
	for _, a := range as {
		key := a.Key
		if h.prefix != "" {
			key = h.prefix + "." + key
		}
		next.attrs[key] = a.Value.Any()
	}
	return next
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	prefix := name
	if h.prefix != "" {
		prefix = h.prefix + "." + name
	}
	next := &slogHandler{prefix: prefix, attrs: make(map[string]any, len(h.attrs))}
	maps.Copy(next.attrs, h.attrs)
	return next
}

// Log returns an *slog.Logger that emits records via host op 1 (log). If ctx
// carries an in-flight invocation, its id is attached as the "invocationId"
// attribute on every record.
func Log(ctx context.Context) *slog.Logger {
	h := &slogHandler{}
	if inv := Invocation(ctx); inv.ID != "" {
		h = &slogHandler{attrs: map[string]any{"invocationId": inv.ID}}
	}
	return slog.New(h)
}
