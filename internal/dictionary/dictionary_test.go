package dictionary

import (
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestApplyDefiniteCorrection(t *testing.T) {
	dict := Dictionary{
		"ja": {
			Definite: []Rule{{
				From:     []string{"Open A I"},
				To:       "OpenAI",
				Priority: 5,
			}},
		},
	}
	transcript := domain.Transcript{
		Language: "ja",
		Text:     "Open A I is here",
		Segments: []domain.Segment{{Text: "Open A I is here"}},
	}
	got := Apply(transcript, dict)
	if got.Text != "OpenAI is here" {
		t.Fatalf("unexpected text: %s", got.Text)
	}
	if got.Segments[0].Text != "OpenAI is here" {
		t.Fatalf("unexpected segment text: %s", got.Segments[0].Text)
	}
}
