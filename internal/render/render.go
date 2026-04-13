package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-transcriber-cli/internal/domain"
)

type RenderInput struct {
	Transcript   domain.Transcript
	SourcePath   string
	Format       domain.OutputFormat
	ChunkingMode domain.ChunkingMode
}

func Transcript(input RenderInput, includeSegments bool) ([]byte, error) {
	switch input.Format {
	case domain.FormatTXT:
		return []byte(input.Transcript.Text), nil
	case domain.FormatMD:
		return renderMarkdown(input, includeSegments), nil
	case domain.FormatJSON:
		return renderJSON(input)
	case domain.FormatSRT:
		return renderSRT(input.Transcript)
	case domain.FormatVTT:
		return renderVTT(input.Transcript)
	default:
		return nil, domain.NewError("format_not_supported", fmt.Sprintf("unsupported format: %s", input.Format), domain.ExitArgs, nil)
	}
}

func renderMarkdown(input RenderInput, includeSegments bool) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "source: %s\n", input.SourcePath)
	fmt.Fprintf(&b, "created_at: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "model: %s\n", input.Transcript.ModelUsed)
	fmt.Fprintf(&b, "language: %s\n", input.Transcript.Language)
	fmt.Fprintf(&b, "duration_sec: %.2f\n", input.Transcript.DurationSec)
	fmt.Fprintf(&b, "partial: %t\n", input.Transcript.Partial)
	fmt.Fprintf(&b, "chunking_mode: %s\n", input.ChunkingMode)
	if len(input.Transcript.Speakers) > 0 {
		fmt.Fprintf(&b, "speakers: %s\n", strings.Join(input.Transcript.Speakers, ", "))
	}
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# Transcript\n\n%s\n", input.Transcript.Text)
	fmt.Fprintf(&b, "\n## Metadata\n\n- warnings: %s\n- output_format: %s\n", warningsJSON(input.Transcript.Warnings), input.Format)
	if includeSegments && len(input.Transcript.Segments) > 0 {
		fmt.Fprintf(&b, "\n## Segments\n\n")
		for _, segment := range input.Transcript.Segments {
			fmt.Fprintf(&b, "- [%.2f - %.2f] %s\n", segment.StartSec, segment.EndSec, segmentLine(segment))
		}
	}
	return b.Bytes()
}

func renderJSON(input RenderInput) ([]byte, error) {
	payload := map[string]any{
		"version": domain.SchemaVersion,
		"input": map[string]any{
			"path":         input.SourcePath,
			"duration_sec": input.Transcript.DurationSec,
		},
		"transcript": input.Transcript,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func renderSRT(t domain.Transcript) ([]byte, error) {
	if len(t.Segments) == 0 {
		return nil, domain.NewError("subtitle_segments_required", "subtitle output requires segments", domain.ExitWrite, nil)
	}
	var b bytes.Buffer
	for i, seg := range t.Segments {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, formatSRTTime(seg.StartSec), formatSRTTime(seg.EndSec), segmentLine(seg))
	}
	return b.Bytes(), nil
}

func renderVTT(t domain.Transcript) ([]byte, error) {
	if len(t.Segments) == 0 {
		return nil, domain.NewError("subtitle_segments_required", "subtitle output requires segments", domain.ExitWrite, nil)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "WEBVTT\n\n")
	for _, seg := range t.Segments {
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", formatVTTTime(seg.StartSec), formatVTTTime(seg.EndSec), segmentLine(seg))
	}
	return b.Bytes(), nil
}

func formatSRTTime(sec float64) string {
	totalMS := int(sec * 1000)
	ms := totalMS % 1000
	totalSeconds := totalMS / 1000
	s := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	m := totalMinutes % 60
	h := totalMinutes / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func formatVTTTime(sec float64) string {
	totalMS := int(sec * 1000)
	ms := totalMS % 1000
	totalSeconds := totalMS / 1000
	s := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	m := totalMinutes % 60
	h := totalMinutes / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func warningsJSON(w []domain.Warning) string {
	data, _ := json.Marshal(w)
	return string(data)
}

func segmentLine(seg domain.Segment) string {
	text := strings.TrimSpace(seg.Text)
	if seg.Speaker == nil || strings.TrimSpace(*seg.Speaker) == "" {
		return text
	}
	return fmt.Sprintf("%s: %s", strings.TrimSpace(*seg.Speaker), text)
}
