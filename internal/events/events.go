package events

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"ai-transcriber-cli/internal/domain"
)

type Envelope struct {
	Type            string `json:"type"`
	ProtocolVersion string `json:"protocol_version"`
	JobID           string `json:"job_id"`
	TS              string `json:"ts"`
	Code            string `json:"code,omitempty"`
}

type Writer interface {
	Emit(eventType string, payload map[string]any) error
}

type JSONLWriter struct {
	mu    sync.Mutex
	out   io.Writer
	jobID string
}

func NewJSONLWriter(out io.Writer, jobID string) *JSONLWriter {
	return &JSONLWriter{out: out, jobID: jobID}
}

func (w *JSONLWriter) Emit(eventType string, payload map[string]any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	base := map[string]any{
		"type":             eventType,
		"protocol_version": domain.ProtocolVersion,
		"job_id":           w.jobID,
		"ts":               time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range payload {
		base[k] = v
	}
	data, err := json.Marshal(base)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w.out, string(data))
	return err
}

type TextWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func NewTextWriter(out io.Writer) *TextWriter {
	return &TextWriter{out: out}
}

func (w *TextWriter) Emit(eventType string, payload map[string]any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	msg, _ := payload["message"].(string)
	if msg == "" {
		msg = eventType
	}
	_, err := fmt.Fprintf(w.out, "%s: %s\n", eventType, msg)
	return err
}

type NopWriter struct{}

func (NopWriter) Emit(string, map[string]any) error { return nil }
