package merge

import (
	"sort"
	"strings"
	"unicode"

	"ai-transcriber-cli/internal/domain"
)

func Merge(model string, chunks []domain.Transcript, offsets []float64) domain.Transcript {
	if len(chunks) == 0 {
		return domain.Transcript{Version: domain.SchemaVersion}
	}
	merged := chunks[0]
	merged.Version = domain.SchemaVersion
	if len(offsets) > 0 {
		for i := range merged.Segments {
			merged.Segments[i].StartSec += offsets[0]
			merged.Segments[i].EndSec += offsets[0]
		}
		for i := range merged.Words {
			merged.Words[i].StartSec += offsets[0]
			merged.Words[i].EndSec += offsets[0]
		}
	}
	speakerSet := make(map[string]struct{}, len(merged.Speakers))
	for _, speaker := range merged.Speakers {
		speakerSet[speaker] = struct{}{}
	}
	for i := 1; i < len(chunks); i++ {
		next := chunks[i]
		text, warning := mergeText(merged.Text, next.Text)
		merged.Text = text
		if warning != nil {
			merged.Warnings = domain.AppendWarning(merged.Warnings, *warning)
		}
		merged.Usage = merged.Usage.Add(next.Usage)
		if next.DurationSec+offsets[i] > merged.DurationSec {
			merged.DurationSec = next.DurationSec + offsets[i]
		}
		merged.Partial = merged.Partial || next.Partial
		for _, seg := range next.Segments {
			seg.StartSec += offsets[i]
			seg.EndSec += offsets[i]
			merged.Segments = append(merged.Segments, seg)
		}
		for _, word := range next.Words {
			word.StartSec += offsets[i]
			word.EndSec += offsets[i]
			merged.Words = append(merged.Words, word)
		}
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
	return merged
}

func mergeText(left, right string) (string, *domain.Warning) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right, nil
	}
	if right == "" {
		return left, nil
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
	max := min(len(suffixRunes), len(prefixRunes))
	for i := 12; i <= max; i++ {
		if string(suffixRunes[len(suffixRunes)-i:]) == string(prefixRunes[:i]) {
			best = i
		}
	}
	if best > 0 {
		return string(leftRunes) + string(rightRunes[best:]), nil
	}
	for i := 12; i <= max; i++ {
		if normalize(string(suffixRunes[len(suffixRunes)-i:])) == normalize(string(prefixRunes[:i])) {
			best = i
		}
	}
	if best > 0 {
		return string(leftRunes) + string(rightRunes[best:]), nil
	}
	return left + "\n\n" + right, &domain.Warning{
		Code:    "merge_overlap_unresolved",
		Message: "chunk overlap could not be resolved cleanly; transcript was joined conservatively",
	}
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
