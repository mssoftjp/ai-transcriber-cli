package merge

import (
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestMergeKeepsUsageWordsAndSpeakers(t *testing.T) {
	speakerA := "speaker_a"
	speakerB := "speaker_b"
	merged := Merge("whisper-1", []domain.Transcript{
		{
			Text: "hello world",
			Words: []domain.WordTiming{{
				StartSec: 0.1,
				EndSec:   0.2,
				Word:     "hello",
			}},
			Speakers: []string{speakerA},
			Usage:    domain.Usage{Type: "tokens", InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		},
		{
			Text: "world again",
			Words: []domain.WordTiming{{
				StartSec: 0.3,
				EndSec:   0.4,
				Word:     "again",
			}},
			Speakers: []string{speakerB},
			Usage:    domain.Usage{Type: "tokens", InputTokens: 7, OutputTokens: 3, TotalTokens: 10},
		},
	}, []float64{0, 10})

	if merged.Usage.InputTokens != 17 || merged.Usage.TotalTokens != 25 {
		t.Fatalf("unexpected merged usage: %#v", merged.Usage)
	}
	if len(merged.Words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(merged.Words))
	}
	if merged.Words[1].StartSec != 10.3 {
		t.Fatalf("expected offset word timing, got %#v", merged.Words[1])
	}
	if len(merged.Speakers) != 2 {
		t.Fatalf("expected speaker union, got %#v", merged.Speakers)
	}
}

func TestMergeSortsAndDedupesSegmentsAndWords(t *testing.T) {
	t.Parallel()

	merged := Merge("whisper-1", []domain.Transcript{
		{
			Text: "alpha bravo",
			Segments: []domain.Segment{
				{StartSec: 5, EndSec: 9, Text: "bravo"},
			},
			Words: []domain.WordTiming{
				{StartSec: 5.1, EndSec: 5.3, Word: "bravo"},
			},
		},
		{
			Text: "bravo charlie",
			Segments: []domain.Segment{
				{StartSec: 0, EndSec: 4, Text: "bravo"},
				{StartSec: 4, EndSec: 8, Text: "charlie"},
			},
			Words: []domain.WordTiming{
				{StartSec: 0.1, EndSec: 0.2, Word: "bravo"},
				{StartSec: 4.1, EndSec: 4.2, Word: "charlie"},
			},
		},
	}, []float64{0, 5})

	if len(merged.Segments) != 2 {
		t.Fatalf("expected deduped segments, got %#v", merged.Segments)
	}
	if merged.Segments[0].Text != "bravo" || merged.Segments[1].Text != "charlie" {
		t.Fatalf("unexpected segment order: %#v", merged.Segments)
	}
	if len(merged.Words) != 2 {
		t.Fatalf("expected deduped words, got %#v", merged.Words)
	}
	if merged.Words[0].Word != "bravo" || merged.Words[1].Word != "charlie" {
		t.Fatalf("unexpected word order: %#v", merged.Words)
	}
}

func TestMergeAddsWarningWhenOverlapCannotBeResolved(t *testing.T) {
	t.Parallel()

	merged := Merge("gpt-4o-transcribe", []domain.Transcript{
		{Text: "今日は晴れです"},
		{Text: "明日は雨かもしれません"},
	}, []float64{0, 30})

	found := false
	for _, warning := range merged.Warnings {
		if warning.Code == "merge_overlap_unresolved" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected merge warning, got %#v", merged.Warnings)
	}
}
