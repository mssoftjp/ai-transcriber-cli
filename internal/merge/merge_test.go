package merge

import (
	"strings"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestMergeKeepsUsageWordsAndSpeakers(t *testing.T) {
	speakerA := "speaker_a"
	speakerB := "speaker_b"
	result := Merge("whisper-1", []domain.Transcript{
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
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 10},
		{Index: 1, StartSec: 10, EndSec: 20},
	})
	merged := result.Transcript

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

	result := Merge("whisper-1", []domain.Transcript{
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
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 10},
		{Index: 1, StartSec: 5, EndSec: 15},
	})
	merged := result.Transcript

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

func TestMergeV2UsesFuzzyBoundaryForShiftedRestart(t *testing.T) {
	t.Parallel()

	left := "本日は健康づくりの話をします。大阪に行くので、今後大阪府において健康づくりを広げます。"
	right := "その中で大阪に行くので、今後大阪府において健康づくりを広げます。次にフレイル予防の話に進みます。"
	result := Merge("gpt-4o-mini-transcribe", []domain.Transcript{
		{Text: left},
		{Text: right},
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 60, OverlapSec: 30},
		{Index: 1, StartSec: 30, EndSec: 90, OverlapSec: 30},
	})

	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected diagnostics for one boundary, got %#v", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Strategy != "v2_trigram" {
		t.Fatalf("expected v2_trigram strategy, got %#v", diag)
	}
	if diag.LocalWindowRunes == 0 {
		t.Fatalf("expected local window diagnostics, got %#v", diag)
	}
	if diag.FallbackUsed {
		t.Fatalf("expected non-fallback merge, got %#v", diag)
	}
	if got := result.Transcript.Text; got == left+"\n\n"+right {
		t.Fatalf("expected overlap trimming, got conservative join: %q", got)
	}
	if countSubstring(result.Transcript.Text, "大阪に行くので") != 1 {
		t.Fatalf("expected duplicated overlap to be reduced, got %q", result.Transcript.Text)
	}
	if countSubstring(result.Transcript.Text, "次にフレイル予防の話に進みます") != 1 {
		t.Fatalf("expected new content to remain, got %q", result.Transcript.Text)
	}
}

func TestMergeV2TrimsLeadingPreambleBeforeOverlap(t *testing.T) {
	t.Parallel()

	left := "今日は内容を一部抜粋してお話しします。フレイルというのは心身の機能が衰え始める状態を指します。"
	right := "皆様、ありがとうございます。今日こちらの内容も一部抜粋してお話しいたします。まずですね、ここだけ覚えてください。フレイルというのは心身の機能が衰え始める状態を指します。次に予防策の話をします。"
	result := Merge("gpt-4o-mini-transcribe", []domain.Transcript{
		{Text: left},
		{Text: right},
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 90, OverlapSec: 30},
		{Index: 1, StartSec: 60, EndSec: 150, OverlapSec: 30},
	})

	if got := countSubstring(result.Transcript.Text, "フレイルというのは心身の機能が衰え始める状態を指します"); got != 1 {
		t.Fatalf("expected preamble case to keep one overlap copy, got %d in %q", got, result.Transcript.Text)
	}
	if countSubstring(result.Transcript.Text, "次に予防策の話をします") != 1 {
		t.Fatalf("expected new content to remain, got %q", result.Transcript.Text)
	}
}

func TestMergeV2CanCutAtClauseBoundary(t *testing.T) {
	t.Parallel()

	left := "こちらが大阪市でして、その下が岸和田とか、泉南市とかあります。"
	right := "こちらが大阪市でして、その下が岸和田とか、ふるさとで有名な泉南市とかあります。次に調査結果へ進みます。"
	result := Merge("gpt-4o-transcribe", []domain.Transcript{
		{Text: left},
		{Text: right},
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 90, OverlapSec: 30},
		{Index: 1, StartSec: 60, EndSec: 150, OverlapSec: 30},
	})

	if result.Diagnostics[0].FallbackUsed {
		t.Fatalf("expected clause-boundary merge without fallback, got %#v", result.Diagnostics[0])
	}
	if countSubstring(result.Transcript.Text, "こちらが大阪市でして、その下が岸和田とか") != 1 {
		t.Fatalf("expected clause overlap to be trimmed, got %q", result.Transcript.Text)
	}
	if countSubstring(result.Transcript.Text, "次に調査結果へ進みます") != 1 {
		t.Fatalf("expected trailing new content to remain, got %q", result.Transcript.Text)
	}
}

func TestMergeV2FallsBackForWeakCandidates(t *testing.T) {
	t.Parallel()

	result := Merge("gpt-4o-transcribe", []domain.Transcript{
		{Text: "今日は晴れです"},
		{Text: "明日は雨かもしれません"},
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 60, OverlapSec: 30},
		{Index: 1, StartSec: 30, EndSec: 90, OverlapSec: 30},
	})

	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected diagnostics for one boundary, got %#v", result.Diagnostics)
	}
	if !result.Diagnostics[0].FallbackUsed {
		t.Fatalf("expected fallback diagnostic, got %#v", result.Diagnostics[0])
	}
	found := false
	for _, warning := range result.Transcript.Warnings {
		if warning.Code == "merge_overlap_unresolved" && countSubstring(warning.Message, "0->1") == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected boundary-specific merge warning, got %#v", result.Transcript.Warnings)
	}
}

func TestMergeV2SkipsSparseRightChunk(t *testing.T) {
	t.Parallel()

	result := Merge("gpt-4o-transcribe", []domain.Transcript{
		{Text: "十分に長い左側テキストです。重複判定のための文脈もあります。"},
		{Text: "あ"},
	}, []domain.ChunkPlan{
		{Index: 0, StartSec: 0, EndSec: 60, OverlapSec: 30},
		{Index: 1, StartSec: 30, EndSec: 90, OverlapSec: 30},
	})

	if result.Diagnostics[0].Strategy != "v2_short_text_fallback" {
		t.Fatalf("expected sparse-text fallback, got %#v", result.Diagnostics[0])
	}
	if !result.Diagnostics[0].FallbackUsed {
		t.Fatalf("expected fallback diagnostic, got %#v", result.Diagnostics[0])
	}
}

func countSubstring(text, needle string) int {
	count := 0
	for {
		idx := strings.Index(text, needle)
		if idx < 0 {
			return count
		}
		count++
		text = text[idx+len(needle):]
	}
}
