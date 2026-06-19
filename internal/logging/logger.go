package logging

import (
	"encoding/json"
	"io"
	"strings"
	"time"
)

type Logger struct {
	out   io.Writer
	level string
}

func New(out io.Writer, level string) *Logger {
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	return &Logger{out: out, level: level}
}

func (l *Logger) Info(message string, fields ...string) {
	l.write("info", message, fields...)
}

func (l *Logger) Error(message string, fields ...string) {
	l.write("error", message, fields...)
}

func (l *Logger) write(level, message string, fields ...string) {
	entry := map[string]string{
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
		"level":   level,
		"message": message,
	}

	for i := 0; i+1 < len(fields); i += 2 {
		entry[fields[i]] = Redact(fields[i+1])
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = l.out.Write(append(encoded, '\n'))
}

func Redact(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "postgres://") ||
		strings.Contains(lower, "postgresql://") {
		return "[REDACTED]"
	}
	return value
}
