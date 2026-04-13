package vad

import (
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestParseSilenceDetect(t *testing.T) {
	t.Parallel()

	output := `
[silencedetect @ 0x0] silence_start: 22.400
[silencedetect @ 0x0] silence_end: 24.000 | silence_duration: 1.600
[silencedetect @ 0x0] silence_start: 51.200
[silencedetect @ 0x0] silence_end: 52.000 | silence_duration: 0.800
`
	silences := parseSilenceDetect(output)
	if len(silences) != 2 {
		t.Fatalf("expected 2 silence spans, got %#v", silences)
	}
	if silences[0].StartSec != 22.4 || silences[0].EndSec != 24 {
		t.Fatalf("unexpected first silence span: %#v", silences[0])
	}
}

func TestOptimizeChunkBoundariesUsesNearbySilence(t *testing.T) {
	t.Parallel()

	chunks := []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 25, OverlapSec: 5},
		{Index: 1, StartSec: 20, EndSec: 45, OverlapSec: 5},
	}
	optimized := optimizeChunkBoundaries(chunks, []silenceSpan{
		{StartSec: 22.4, EndSec: 24.0},
	})
	if optimized[0].EndSec != 23.2 {
		t.Fatalf("expected first chunk to end at silence midpoint, got %#v", optimized[0])
	}
	if optimized[1].StartSec != 18.2 {
		t.Fatalf("expected second chunk to keep overlap around new boundary, got %#v", optimized[1])
	}
}
