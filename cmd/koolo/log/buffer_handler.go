package log

import (
	"context"
	"log/slog"
	"sync"
)

// LogEntry represents a single log message for the WebUI
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

// BufferHandler is an slog.Handler that also writes to a callback function
type BufferHandler struct {
	wrapped  slog.Handler
	callback func(LogEntry)
	source   string
	attrs    []slog.Attr
	mu       sync.Mutex
}

// NewBufferHandler creates a handler that wraps an existing handler
// and calls the callback function for each log entry
func NewBufferHandler(wrapped slog.Handler, source string, callback func(LogEntry)) *BufferHandler {
	return &BufferHandler{
		wrapped:  wrapped,
		callback: callback,
		source:   source,
	}
}

// Enabled implements slog.Handler
func (h *BufferHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.wrapped.Enabled(ctx, level)
}

// Handle implements slog.Handler
func (h *BufferHandler) Handle(ctx context.Context, r slog.Record) error {
	// Build the message with attributes
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != "" {
			msg += " " + a.Key + "=" + a.Value.String()
		}
		return true
	})

	// Add any handler-level attrs
	for _, a := range h.attrs {
		if a.Key != "" {
			msg += " " + a.Key + "=" + a.Value.String()
		}
	}

	// Create log entry and call callback
	if h.callback != nil {
		entry := LogEntry{
			Timestamp: r.Time.Format("15:04:05"),
			Level:     r.Level.String(),
			Message:   msg,
			Source:    h.source,
		}
		h.callback(entry)
	}

	// Pass to wrapped handler
	return h.wrapped.Handle(ctx, r)
}

// WithAttrs implements slog.Handler
func (h *BufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()

	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	newHandler := &BufferHandler{
		wrapped:  h.wrapped.WithAttrs(attrs),
		callback: h.callback,
		source:   h.source,
		attrs:    newAttrs,
	}
	return newHandler
}

// WithGroup implements slog.Handler
func (h *BufferHandler) WithGroup(name string) slog.Handler {
	return &BufferHandler{
		wrapped:  h.wrapped.WithGroup(name),
		callback: h.callback,
		source:   h.source,
		attrs:    h.attrs,
	}
}
