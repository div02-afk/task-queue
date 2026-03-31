package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
)

type RuntimeConfig struct {
	level           slog.Level
	persistWasmLogs bool
	mirrorWasmLogs  bool
	wasmLogDir      string
}

var runtimeConfig = RuntimeConfig{
	level:           slog.LevelInfo,
	persistWasmLogs: false,
	mirrorWasmLogs:  true,
	wasmLogDir:      "runtime-logs/wasm",
}

func Setup(cfg *config.LoggingConfig) {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: replaceBuiltInAttrs}
	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(os.Stderr, options)
	} else {
		handler = NewPrettyHandler(os.Stderr, &PrettyHandlerOptions{Level: level})
	}

	slog.SetDefault(slog.New(handler))
	runtimeConfig = RuntimeConfig{
		level:           level,
		persistWasmLogs: cfg.PersistWasmLogs,
		mirrorWasmLogs:  cfg.MirrorWasmLogs,
		wasmLogDir:      cfg.WasmLogDir,
	}
}

func Component(name string) *slog.Logger {
	return slog.Default().With("component", name)
}

func Config() RuntimeConfig {
	return runtimeConfig
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
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

func replaceBuiltInAttrs(_ []string, attr slog.Attr) slog.Attr {
	switch attr.Key {
	case slog.TimeKey:
		if attr.Value.Kind() == slog.KindTime {
			return slog.String(slog.TimeKey, attr.Value.Time().Format("2006-01-02 15:04:05"))
		}
	case slog.LevelKey:
		return slog.String(slog.LevelKey, formatLevel(attr.Value))
	}
	return attr
}

func formatLevel(value slog.Value) string {
	level := slog.LevelInfo
	switch value.Kind() {
	case slog.KindAny:
		if parsed, ok := value.Any().(slog.Level); ok {
			level = parsed
		}
	case slog.KindInt64:
		level = slog.Level(value.Int64())
	}

	switch {
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO "
	case level < slog.LevelError:
		return "WARN "
	default:
		return "ERROR"
	}
}

type PrettyHandlerOptions struct {
	Level slog.Leveler
}

type PrettyHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func NewPrettyHandler(writer io.Writer, opts *PrettyHandlerOptions) *PrettyHandler {
	level := slog.Leveler(slog.LevelInfo)
	if opts != nil && opts.Level != nil {
		level = opts.Level
	}

	return &PrettyHandler{
		writer: writer,
		level:  level,
		attrs:  nil,
		groups: nil,
		mu:     &sync.Mutex{},
	}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *PrettyHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make([]string, 0, len(h.attrs)+8)

	component := ""
	appendAttr := func(attr slog.Attr) {
		attr.Value = attr.Value.Resolve()
		if attr.Equal(slog.Attr{}) {
			return
		}

		key := qualifyKey(h.groups, attr.Key)
		value := formatValue(attr.Value)
		if key == "component" {
			component = value
			return
		}
		fields = append(fields, key+"="+value)
	}

	for _, attr := range h.attrs {
		appendAttr(attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendAttr(attr)
		return true
	})

	var b strings.Builder
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = timeNow()
	}
	b.WriteString(timestamp.Format("2006-01-02 15:04:05"))
	b.WriteByte(' ')
	b.WriteString(formatLevel(slog.AnyValue(record.Level)))
	b.WriteByte(' ')

	if component != "" {
		b.WriteByte('[')
		b.WriteString(component)
		b.WriteByte(']')
		b.WriteByte(' ')
	}

	b.WriteString(record.Message)
	if len(fields) > 0 {
		b.WriteString(" | ")
		b.WriteString(strings.Join(fields, " "))
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, b.String())
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	combined = append(combined, attrs...)
	return &PrettyHandler{
		writer: h.writer,
		level:  h.level,
		attrs:  combined,
		groups: append([]string(nil), h.groups...),
		mu:     h.mu,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	groups := append([]string(nil), h.groups...)
	groups = append(groups, name)
	return &PrettyHandler{
		writer: h.writer,
		level:  h.level,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: groups,
		mu:     h.mu,
	}
}

func qualifyKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(append(append([]string(nil), groups...), key), ".")
}

func formatValue(value slog.Value) string {
	value = value.Resolve()

	switch value.Kind() {
	case slog.KindString:
		return quoteIfNeeded(value.String())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format("2006-01-02 15:04:05")
	case slog.KindAny:
		return quoteIfNeeded(fmt.Sprint(value.Any()))
	default:
		return quoteIfNeeded(value.String())
	}
}

func quoteIfNeeded(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n=\"") {
		return strconv.Quote(value)
	}
	return value
}

var timeNow = func() time.Time {
	return time.Now()
}
