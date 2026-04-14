package merge

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ai-transcriber-cli/internal/domain"
)

type Result struct {
	Transcript  domain.Transcript
	Diagnostics []domain.MergeBoundaryDiagnostic
}

func Merge(model string, chunks []domain.Transcript, plans []domain.ChunkPlan) Result {
	if len(chunks) == 0 {
		return Result{Transcript: domain.Transcript{Version: domain.SchemaVersion}}
	}

	merged := chunks[0]
	merged.Version = domain.SchemaVersion
	firstPlan := chunkPlanAt(plans, 0)
	applyTimingOffset(&merged, firstPlan.StartSec)

	speakerSet := make(map[string]struct{}, len(merged.Speakers))
	for _, speaker := range merged.Speakers {
		speakerSet[speaker] = struct{}{}
	}

	profile, _ := domain.ModelProfileFor(model)
	diagnostics := make([]domain.MergeBoundaryDiagnostic, 0, max(0, len(chunks)-1))
	for i := 1; i < len(chunks); i++ {
		next := chunks[i]
		currentPlan := chunkPlanAt(plans, i)
		prevPlan := chunkPlanAt(plans, i-1)

		var (
			text       string
			warning    *domain.Warning
			diagnostic domain.MergeBoundaryDiagnostic
		)
		if profile.UseOverlapMergeV2 {
			text, warning, diagnostic = mergeTextV2(merged.Text, next.Text, prevPlan, currentPlan)
		} else {
			text, warning, diagnostic = mergeTextLegacy(merged.Text, next.Text, i-1, prevPlan, currentPlan)
		}
		merged.Text = text
		if warning != nil {
			merged.Warnings = domain.AppendWarning(merged.Warnings, *warning)
		}
		diagnostics = append(diagnostics, diagnostic)

		merged.Usage = merged.Usage.Add(next.Usage)
		if next.DurationSec+currentPlan.StartSec > merged.DurationSec {
			merged.DurationSec = next.DurationSec + currentPlan.StartSec
		}
		merged.Partial = merged.Partial || next.Partial
		appendShiftedSegments(&merged, next.Segments, currentPlan.StartSec)
		appendShiftedWords(&merged, next.Words, currentPlan.StartSec)
		for _, speaker := range next.Speakers {
			if _, ok := speakerSet[speaker]; ok {
				continue
			}
			speakerSet[speaker] = struct{}{}
			merged.Speakers = append(merged.Speakers, speaker)
		}
		merged.Warnings = append(merged.Warnings, next.Warnings...)
	}

	merged.Segments = sortAndDedupeSegments(merged.Segments)
	merged.Words = sortAndDedupeWords(merged.Words)
	if merged.Text == "" && len(merged.Segments) > 0 {
		texts := make([]string, 0, len(merged.Segments))
		for _, seg := range merged.Segments {
			texts = append(texts, strings.TrimSpace(seg.Text))
		}
		merged.Text = strings.Join(texts, " ")
	}
	merged.Warnings = domain.DedupeWarnings(merged.Warnings)

	return Result{Transcript: merged, Diagnostics: diagnostics}
}

func mergeTextLegacy(left, right string, boundaryIndex int, leftPlan, rightPlan domain.ChunkPlan) (string, *domain.Warning, domain.MergeBoundaryDiagnostic) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	diagnostic := newDiagnostic(boundaryIndex, leftPlan, rightPlan)
	diagnostic.Strategy = "legacy"

	if left == "" {
		diagnostic.Strategy = "left_empty"
		return right, nil, diagnostic
	}
	if right == "" {
		diagnostic.Strategy = "right_empty"
		return left, nil, diagnostic
	}

	leftRunes := []rune(left)
	rightRunes := []rune(right)
	suffixRunes := leftRunes
	if len(suffixRunes) > 300 {
		suffixRunes = suffixRunes[len(suffixRunes)-300:]
	}
	prefixRunes := rightRunes
	if len(prefixRunes) > 300 {
		prefixRunes = prefixRunes[:300]
	}

	best := 0
	maxRunes := min(len(suffixRunes), len(prefixRunes))
	for i := 12; i <= maxRunes; i++ {
		if string(suffixRunes[len(suffixRunes)-i:]) == string(prefixRunes[:i]) {
			best = i
		}
	}
	if best > 0 {
		diagnostic.Strategy = "legacy_exact"
		diagnostic.CandidateCount = 1
		diagnostic.ChosenCutRune = best
		return string(leftRunes) + string(rightRunes[best:]), nil, diagnostic
	}

	for i := 12; i <= maxRunes; i++ {
		if normalize(string(suffixRunes[len(suffixRunes)-i:])) == normalize(string(prefixRunes[:i])) {
			best = i
		}
	}
	if best > 0 {
		diagnostic.Strategy = "legacy_normalized"
		diagnostic.CandidateCount = 1
		diagnostic.ChosenCutRune = best
		return string(leftRunes) + string(rightRunes[best:]), nil, diagnostic
	}

	diagnostic.Strategy = "legacy_fallback"
	diagnostic.FallbackUsed = true
	return left + "\n\n" + right, unresolvedWarning(boundaryIndex), diagnostic
}

func applyTimingOffset(transcript *domain.Transcript, offset float64) {
	if offset == 0 {
		return
	}
	for i := range transcript.Segments {
		transcript.Segments[i].StartSec += offset
		transcript.Segments[i].EndSec += offset
	}
	for i := range transcript.Words {
		transcript.Words[i].StartSec += offset
		transcript.Words[i].EndSec += offset
	}
}

func appendShiftedSegments(merged *domain.Transcript, segments []domain.Segment, offset float64) {
	for _, seg := range segments {
		seg.StartSec += offset
		seg.EndSec += offset
		merged.Segments = append(merged.Segments, seg)
	}
}

func appendShiftedWords(merged *domain.Transcript, words []domain.WordTiming, offset float64) {
	for _, word := range words {
		word.StartSec += offset
		word.EndSec += offset
		merged.Words = append(merged.Words, word)
	}
}

func unresolvedWarning(boundaryIndex int) *domain.Warning {
	return &domain.Warning{
		Code:    "merge_overlap_unresolved",
		Message: "chunk overlap at boundary " + formatBoundary(boundaryIndex) + " could not be resolved cleanly; transcript was joined conservatively",
	}
}

func formatBoundary(boundaryIndex int) string {
	return strings.Join([]string{itoa(boundaryIndex), "->", itoa(boundaryIndex + 1)}, "")
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func chunkPlanAt(plans []domain.ChunkPlan, index int) domain.ChunkPlan {
	if index >= 0 && index < len(plans) {
		return plans[index]
	}
	return domain.ChunkPlan{Index: index}
}

func newDiagnostic(boundaryIndex int, leftPlan, rightPlan domain.ChunkPlan) domain.MergeBoundaryDiagnostic {
	return domain.MergeBoundaryDiagnostic{
		BoundaryIndex:    boundaryIndex,
		LeftChunkIndex:   leftPlan.Index,
		RightChunkIndex:  rightPlan.Index,
		OverlapSec:       rightPlan.OverlapSec,
		LeftDurationSec:  chunkDuration(leftPlan),
		RightDurationSec: chunkDuration(rightPlan),
	}
}

func chunkDuration(plan domain.ChunkPlan) float64 {
	if plan.EndSec <= plan.StartSec {
		return 0
	}
	return plan.EndSec - plan.StartSec
}

func sortAndDedupeSegments(segments []domain.Segment) []domain.Segment {
	if len(segments) == 0 {
		return nil
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].StartSec == segments[j].StartSec {
			return segments[i].EndSec < segments[j].EndSec
		}
		return segments[i].StartSec < segments[j].StartSec
	})
	out := make([]domain.Segment, 0, len(segments))
	for _, candidate := range segments {
		if len(out) == 0 || shouldAppendSegment(out[len(out)-1], candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func sortAndDedupeWords(words []domain.WordTiming) []domain.WordTiming {
	if len(words) == 0 {
		return nil
	}
	sort.SliceStable(words, func(i, j int) bool {
		if words[i].StartSec == words[j].StartSec {
			return words[i].EndSec < words[j].EndSec
		}
		return words[i].StartSec < words[j].StartSec
	})
	out := make([]domain.WordTiming, 0, len(words))
	for _, candidate := range words {
		if len(out) == 0 || shouldAppendWord(out[len(out)-1], candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func shouldAppendSegment(last, candidate domain.Segment) bool {
	if candidate.StartSec >= last.EndSec {
		return true
	}
	lastText := normalize(last.Text)
	candidateText := normalize(candidate.Text)
	if lastText == candidateText {
		return false
	}
	if candidate.StartSec >= last.StartSec && candidate.EndSec <= last.EndSec && strings.Contains(lastText, candidateText) {
		return false
	}
	return true
}

func shouldAppendWord(last, candidate domain.WordTiming) bool {
	if candidate.StartSec > last.EndSec {
		return true
	}
	if normalize(last.Word) == normalize(candidate.Word) {
		return false
	}
	return true
}

func normalize(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return r
	}, text)
}
