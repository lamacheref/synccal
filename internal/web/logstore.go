package web

import (
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogEntry is a single captured structured log line.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogStore is a thread-safe bounded ring buffer of recent log entries.
type LogStore struct {
	mu    sync.RWMutex
	buf   []LogEntry
	max   int
	next  int
	count int
}

func NewLogStore(max int) *LogStore {
	return &LogStore{buf: make([]LogEntry, max), max: max}
}

func (l *LogStore) Append(ts time.Time, level, message string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf[l.next] = LogEntry{Timestamp: ts, Level: level, Message: message, Fields: fields}
	l.next = (l.next + 1) % l.max
	if l.count < l.max {
		l.count++
	}
}

// List returns entries newest-first, optionally filtered by minimum level.
func (l *LogStore) List(level string, limit int) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	minLevel := zapcore.InfoLevel
	if level != "" {
		if lvl, err := zapcore.ParseLevel(level); err == nil {
			minLevel = lvl
		}
	}
	if limit <= 0 || limit > l.count {
		limit = l.count
	}

	out := make([]LogEntry, 0, limit)
	idx := (l.next - 1 + l.max) % l.max
	for i := 0; i < l.count && len(out) < limit; i++ {
		e := l.buf[idx]
		lvl, err := zapcore.ParseLevel(e.Level)
		if err == nil && lvl >= minLevel {
			out = append(out, e)
		}
		idx = (idx - 1 + l.max) % l.max
	}
	return out
}

// Core returns a zapcore.Core that copies every logged entry into the store
// while still delegating to the wrapped core for the actual output.
func (l *LogStore) Core(wrapped zapcore.Core) zapcore.Core {
	return &logCore{store: l, level: zapcore.DebugLevel, writer: wrapped}
}

type logCore struct {
	store  *LogStore
	level  zapcore.Level
	fields []zapcore.Field
	writer zapcore.Core
}

func (c *logCore) Enabled(l zapcore.Level) bool {
	return l >= c.level
}

func (c *logCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	clone.fields = append(append([]zapcore.Field{}, c.fields...), fields...)
	return &clone
}

func (c *logCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *logCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	all := append(append([]zapcore.Field{}, c.fields...), fields...)
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range all {
		f.AddTo(enc)
	}
	c.store.Append(ent.Time, ent.Level.String(), ent.Message, enc.Fields)
	if c.writer != nil {
		return c.writer.Write(ent, fields)
	}
	return nil
}

func (c *logCore) Sync() error {
	if c.writer != nil {
		return c.writer.Sync()
	}
	return nil
}

// Ensure compile-time interface satisfaction.
var _ zapcore.Core = (*logCore)(nil)
var _ = zap.L()
