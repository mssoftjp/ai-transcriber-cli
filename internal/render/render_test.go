package render

import (
	"strings"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestMarkdownIncludesSegmentsWhenRequested(t *testing.T) {
	t.Parallel()

	speaker := "speaker_0"
	data, err := Transcript(RenderInput{
		SourcePath: "/tmp/input.mp3",
		Format:     domain.FormatMD,
		Transcript: domain.Transcript{
			ModelUsed:   "whisper-1",
			Language:    "ja",
			DurationSec: 12.3,
			Text:        "hello",
			Speakers:    []string{speaker},
			Segments: []domain.Segment{{
				ID:       "seg-1",
				StartSec: 0,
				EndSec:   1,
				Text:     "hello",
				Speaker:  &speaker,
			}},
		},
	}, true)
	if err != nil {
		t.Fatalf("Transcript() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "## Segments") {
		t.Fatalf("expected markdown output to contain segments section")
	}
	if !strings.Contains(text, "speaker_0: hello") {
		t.Fatalf("expected markdown output to contain speaker-aware segment text, got %q", text)
	}
	if !strings.Contains(text, "speakers: speaker_0") {
		t.Fatalf("expected markdown front matter to include speakers, got %q", text)
	}
}

func TestSubtitleRenderIncludesSpeakerPrefix(t *testing.T) {
	t.Parallel()

	speaker := "agent"
	data, err := Transcript(RenderInput{
		Format: domain.FormatSRT,
		Transcript: domain.Transcript{
			Segments: []domain.Segment{{
				StartSec: 0,
				EndSec:   1.5,
				Text:     "hello",
				Speaker:  &speaker,
			}},
		},
	}, false)
	if err != nil {
		t.Fatalf("Transcript() error = %v", err)
	}
	if !strings.Contains(string(data), "agent: hello") {
		t.Fatalf("expected speaker prefix in subtitle output, got %q", string(data))
	}
}
