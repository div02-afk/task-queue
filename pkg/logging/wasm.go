package logging

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type WasmTaskLogSink struct {
	logger     *slog.Logger
	logPath    string
	file       *os.File
	mirrorLogs bool
	stdoutBuf  bytes.Buffer
	stderrBuf  bytes.Buffer
	stdout     io.Writer
	stderr     io.Writer
	closeOnce  sync.Once
	mu         sync.Mutex
}

func NewWasmTaskLogSink(logger *slog.Logger, taskName string, taskID string) (*WasmTaskLogSink, error) {
	cfg := Config()

	sink := &WasmTaskLogSink{
		logger:     logger.With("task_name", taskName, "task_id", taskID, "log_type", "wasm"),
		mirrorLogs: cfg.mirrorWasmLogs,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
	}

	if !cfg.persistWasmLogs {
		return sink, nil
	}

	logDir := filepath.Join(cfg.wasmLogDir, sanitizePathSegment(taskName))
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logDir, sanitizePathSegment(taskID)+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	sink.file = file
	sink.logPath = logPath
	sink.logger.Info("persisting wasm logs", "path", logPath)
	return sink, nil
}

func (s *WasmTaskLogSink) Stdout() io.Writer {
	return wasmStreamWriter{sink: s, stream: "stdout"}
}

func (s *WasmTaskLogSink) Stderr() io.Writer {
	return wasmStreamWriter{sink: s, stream: "stderr"}
}

func (s *WasmTaskLogSink) LogPath() string {
	return s.logPath
}

func (s *WasmTaskLogSink) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.flushBuffer("stdout", &s.stdoutBuf)
		s.flushBuffer("stderr", &s.stderrBuf)

		if s.file != nil {
			closeErr = s.file.Close()
		}
	})
	return closeErr
}

type wasmStreamWriter struct {
	sink   *WasmTaskLogSink
	stream string
}

func (w wasmStreamWriter) Write(p []byte) (int, error) {
	return w.sink.write(w.stream, p)
}

func (s *WasmTaskLogSink) write(stream string, p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mirrorLogs {
		mirror := s.stdout
		if stream == "stderr" {
			mirror = s.stderr
		}
		if mirror != nil {
			if _, err := mirror.Write(p); err != nil {
				return 0, err
			}
		}
	}

	buf := &s.stdoutBuf
	if stream == "stderr" {
		buf = &s.stderrBuf
	}
	if _, err := buf.Write(p); err != nil {
		return 0, err
	}

	s.flushCompleteLines(stream, buf)
	return len(p), nil
}

func (s *WasmTaskLogSink) flushCompleteLines(stream string, buf *bytes.Buffer) {
	for {
		data := buf.Bytes()
		newlineIndex := bytes.IndexByte(data, '\n')
		if newlineIndex < 0 {
			return
		}

		line := string(data[:newlineIndex])
		buf.Next(newlineIndex + 1)
		s.emit(stream, strings.TrimRight(line, "\r"))
	}
}

func (s *WasmTaskLogSink) flushBuffer(stream string, buf *bytes.Buffer) {
	if buf.Len() == 0 {
		return
	}

	line := strings.TrimRight(buf.String(), "\r\n")
	buf.Reset()
	if line != "" {
		s.emit(stream, line)
	}
}

func (s *WasmTaskLogSink) emit(stream string, line string) {
	if line == "" {
		return
	}

	s.logger.Info("wasm output", "stream", stream, "output", line)
	if s.file != nil {
		_, _ = fmt.Fprintf(s.file, "%s [%s] %s\n", time.Now().Format(time.RFC3339Nano), stream, line)
	}
}

func sanitizePathSegment(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
