package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"ai-transcriber-cli/internal/domain"
)

type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
)

type Logger struct {
	mu     sync.Mutex
	level  Level
	format domain.LogFormat
	out    io.Writer
}

func New(out io.Writer, level Level, format domain.LogFormat) *Logger {
	return &Logger{out: out, level: level, format: format}
}

func ParseLevel(quiet, verbose bool) (Level, error) {
	if quiet && verbose {
		return "", fmt.Errorf("--quiet and --verbose cannot be used together")
	}
	if quiet {
		return LevelError, nil
	}
	if verbose {
		return LevelDebug, nil
	}
	return LevelInfo, nil
}

func (l *Logger) enabled(level Level) bool {
	order := map[Level]int{
		LevelError: 0,
		LevelWarn:  1,
		LevelInfo:  2,
		LevelDebug: 3,
	}
	return order[level] <= order[l.level]
}

func (l *Logger) log(level Level, msg string, attrs map[string]any) {
	if !l.enabled(level) {
		return
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	attrs = sanitize(attrs, level)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.format == domain.LogFormatJSON {
		payload := map[string]any{
			"ts":      time.Now().UTC().Format(time.RFC3339),
			"level":   level,
			"message": msg,
		}
		for k, v := range attrs {
			payload[k] = v
		}
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintln(l.out, string(data))
		return
	}
	if len(attrs) == 0 {
		_, _ = fmt.Fprintf(l.out, "[%s] %s\n", strings.ToUpper(string(level)), msg)
		return
	}
	data, _ := json.Marshal(attrs)
	_, _ = fmt.Fprintf(l.out, "[%s] %s %s\n", strings.ToUpper(string(level)), msg, string(data))
}

func sanitize(attrs map[string]any, level Level) map[string]any {
	out := map[string]any{}
	for k, v := range attrs {
		if strings.Contains(strings.ToLower(k), "key") {
			continue
		}
		if strings.Contains(strings.ToLower(k), "text") && level != LevelDebug {
			continue
		}
		out[k] = v
	}
	return out
}

func (l *Logger) Error(msg string, attrs map[string]any) { l.log(LevelError, msg, attrs) }
func (l *Logger) Warn(msg string, attrs map[string]any)  { l.log(LevelWarn, msg, attrs) }
func (l *Logger) Info(msg string, attrs map[string]any)  { l.log(LevelInfo, msg, attrs) }
func (l *Logger) Debug(msg string, attrs map[string]any) { l.log(LevelDebug, msg, attrs) }
