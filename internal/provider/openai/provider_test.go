package openai

import (
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestParseDiarizedResponse(t *testing.T) {
	t.Parallel()

	raw := `{
		"text":"hello there",
		"language":"en",
		"duration":12.3,
		"segments":[
			{"id":"seg-1","start":0.0,"end":1.2,"text":"hello","speaker":"speaker_0"},
			{"id":"seg-2","start":1.2,"end":2.4,"text":"there","speaker":"speaker_1"}
		]
	}`
	transcript, ok := parseDiarizedResponse(raw, "gpt-4o-transcribe-diarize")
	if !ok {
		t.Fatal("expected diarized response to parse")
	}
	if transcript.Text != "hello there" || transcript.Language != "en" {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
	if len(transcript.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", transcript.Segments)
	}
	if transcript.Segments[0].Speaker == nil || *transcript.Segments[0].Speaker != "speaker_0" {
		t.Fatalf("expected speaker on first segment, got %#v", transcript.Segments[0])
	}
	if len(transcript.Speakers) != 2 {
		t.Fatalf("expected speakers list, got %#v", transcript.Speakers)
	}
	if transcript.Speakers[0] != "speaker_0" || transcript.Speakers[1] != "speaker_1" {
		t.Fatalf("expected stable speaker order, got %#v", transcript.Speakers)
	}
}

func TestUsageFromVerboseCapturesDurationSeconds(t *testing.T) {
	t.Parallel()

	usage := usageFromVerbose(openai.TranscriptionVerboseUsage{
		Seconds: 12.34,
		Type:    "duration",
	})
	if usage.Type != "duration" {
		t.Fatalf("Type = %q, want duration", usage.Type)
	}
	if usage.Seconds != 12.34 {
		t.Fatalf("Seconds = %v, want 12.34", usage.Seconds)
	}
}

func TestProviderModelNameMapsWhisperTimestampAlias(t *testing.T) {
	t.Parallel()

	if got := providerModelName("whisper-1-ts"); got != "whisper-1" {
		t.Fatalf("providerModelName(whisper-1-ts) = %q, want whisper-1", got)
	}
	if got := providerModelName("gpt-4o-transcribe"); got != "gpt-4o-transcribe" {
		t.Fatalf("providerModelName passthrough = %q", got)
	}
}
