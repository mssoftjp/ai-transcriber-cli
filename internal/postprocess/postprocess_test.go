package postprocess

import (
	"context"
	"errors"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

type fakeResponseClient struct {
	result responseResult
	err    error
}

func (f fakeResponseClient) Create(context.Context, string, string, string) (responseResult, error) {
	if f.err != nil {
		return responseResult{}, f.err
	}
	return f.result, nil
}

func TestApplyNormalizesWhitespaceAndAddsWarnings(t *testing.T) {
	t.Parallel()

	svc := NewServiceForTest(fakeResponseClient{
		result: responseResult{Text: "first line\n\nsecond line corrected"},
	}, "gpt-4o-mini")
	transcript, err := svc.Apply(context.Background(), domain.Transcript{
		Text: "  first   line \n\n second\tline  ",
		Segments: []domain.Segment{
			{Text: "  first   seg  "},
		},
		Words: []domain.WordTiming{
			{StartSec: 0.1, EndSec: 0.2, Word: "first"},
		},
	}, true, "gpt-4o-mini", "cleanup only")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if transcript.Text != "first line\n\nsecond line corrected" {
		t.Fatalf("unexpected normalized text: %q", transcript.Text)
	}
	if transcript.Segments[0].Text != "first seg" {
		t.Fatalf("unexpected normalized segment text: %q", transcript.Segments[0].Text)
	}
	codes := map[string]bool{}
	for _, warning := range transcript.Warnings {
		codes[warning.Code] = true
	}
	if !codes["postprocess_applied"] {
		t.Fatalf("expected postprocess_applied warning, got %#v", transcript.Warnings)
	}
	if !codes["postprocess_word_alignment_unchecked"] {
		t.Fatalf("expected word alignment warning, got %#v", transcript.Warnings)
	}
	if !codes["postprocess_ai_applied"] {
		t.Fatalf("expected AI-applied warning, got %#v", transcript.Warnings)
	}
}

func TestApplyFailsOpenWhenClientErrors(t *testing.T) {
	t.Parallel()

	svc := NewServiceForTest(fakeResponseClient{err: errors.New("boom")}, "gpt-4o-mini")
	transcript, err := svc.Apply(context.Background(), domain.Transcript{
		Text: "  first   line \n second\tline  ",
	}, true, "", "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if transcript.Text != "first line\n\nsecond line" {
		t.Fatalf("unexpected fail-open text: %q", transcript.Text)
	}
	found := false
	for _, warning := range transcript.Warnings {
		if warning.Code == "postprocess_failed_open" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fail-open warning, got %#v", transcript.Warnings)
	}
}
