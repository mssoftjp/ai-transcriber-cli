package merge

import (
	"math"
	"slices"
	"strings"

	"ai-transcriber-cli/internal/domain"
)

const (
	minContiguousOverlapRunes = 12
	minNormalizedTextRunes    = 18
	minLeftTailRunes          = 120
	maxLeftTailRunes          = 900
	minLocalWindowRunes       = 100
	maxLocalWindowRunes       = 300
	minCandidateStepRunes     = 24
	minOverlapScore           = 0.22
	tieScoreEpsilon           = 0.015
)

func mergeTextV2(left, right string, leftPlan, rightPlan domain.ChunkPlan) (string, *domain.Warning, domain.MergeBoundaryDiagnostic) {
	boundaryIndex := max(0, rightPlan.Index-1)
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	diagnostic := newDiagnostic(boundaryIndex, leftPlan, rightPlan)
	diagnostic.Strategy = "v2"

	if left == "" {
		diagnostic.Strategy = "left_empty"
		return right, nil, diagnostic
	}
	if right == "" {
		diagnostic.Strategy = "right_empty"
		return left, nil, diagnostic
	}

	if merged, cutRune, strategy, ok := resolveContiguousOverlap(left, right); ok {
		diagnostic.Strategy = strategy
		diagnostic.CandidateCount = 1
		diagnostic.ChosenCutRune = cutRune
		return merged, nil, diagnostic
	}

	rightRunes := []rune(right)
	normalizedRight := []rune(normalize(right))
	normalizedLeft := []rune(normalize(left))
	rightDuration := chunkDuration(rightPlan)
	if len(normalizedRight) < minNormalizedTextRunes || rightDuration <= 0 || rightPlan.OverlapSec <= 0 {
		diagnostic.Strategy = "v2_short_text_fallback"
		diagnostic.FallbackUsed = true
		return left + "\n\n" + right, unresolvedWarning(boundaryIndex), diagnostic
	}

	avgRightRate := float64(len(normalizedRight)) / rightDuration
	if avgRightRate < 0.1 {
		diagnostic.Strategy = "v2_sparse_text_fallback"
		diagnostic.FallbackUsed = true
		return left + "\n\n" + right, unresolvedWarning(boundaryIndex), diagnostic
	}
	estimatedOverlapRunes := clampInt(int(math.Round(avgRightRate*rightPlan.OverlapSec)), minNormalizedTextRunes, len(normalizedRight))
	leftTailSize := clampInt(int(math.Round(float64(estimatedOverlapRunes)*2.0)), minLeftTailRunes, maxLeftTailRunes)
	if len(normalizedLeft) < leftTailSize {
		leftTailSize = len(normalizedLeft)
	}
	leftTail := string(normalizedLeft[len(normalizedLeft)-leftTailSize:])
	localWindowRunes := clampInt(int(math.Round(float64(estimatedOverlapRunes)*1.0)), minLocalWindowRunes, maxLocalWindowRunes)

	avgRawRate := float64(len(rightRunes)) / rightDuration
	center := int(math.Round(avgRawRate * rightPlan.OverlapSec))
	windowStart := clampInt(int(math.Round(float64(center)*0.3)), 0, len(rightRunes))
	windowEnd := clampInt(int(math.Round(float64(center)*2.0)), 0, len(rightRunes))
	if estimatedUpper := int(math.Round(float64(center) * 3.0)); estimatedUpper > windowEnd {
		windowEnd = clampInt(estimatedUpper, 0, len(rightRunes))
	}
	if windowEnd <= windowStart {
		windowEnd = min(len(rightRunes), max(windowStart+minCandidateStepRunes, center+minCandidateStepRunes))
	}
	diagnostic.EstimatedOverlapRunes = estimatedOverlapRunes
	diagnostic.LeftTailRunes = leftTailSize
	diagnostic.LocalWindowRunes = localWindowRunes
	diagnostic.SearchWindowStartRune = windowStart
	diagnostic.SearchWindowEndRune = windowEnd

	candidates := candidateCutPoints(rightRunes, windowStart, windowEnd)
	diagnostic.CandidateCount = len(candidates)
	if len(candidates) == 0 {
		diagnostic.Strategy = "v2_no_candidates_fallback"
		diagnostic.FallbackUsed = true
		return left + "\n\n" + right, unresolvedWarning(boundaryIndex), diagnostic
	}

	bestCut := -1
	bestOverlap := 0.0
	bestNovelty := 0.0
	bestCombined := -1.0
	secondBestCombined := -1.0
	for _, cut := range candidates {
		beforeStart := max(0, cut-localWindowRunes)
		discarded := normalize(string(rightRunes[beforeStart:cut]))
		if len([]rune(discarded)) < minContiguousOverlapRunes {
			continue
		}
		keptEnd := min(len(rightRunes), cut+localWindowRunes)
		keptHead := normalize(string(rightRunes[cut:keptEnd]))
		overlapScore := trigramJaccard(discarded, leftTail)
		if overlapScore < minOverlapScore {
			continue
		}
		noveltyScore := trigramJaccard(keptHead, leftTail)
		combinedScore := overlapScore - noveltyScore
		if combinedScore > bestCombined+tieScoreEpsilon || (math.Abs(combinedScore-bestCombined) <= tieScoreEpsilon && (bestCut == -1 || cut < bestCut)) {
			if bestCombined > secondBestCombined {
				secondBestCombined = bestCombined
			}
			bestCut = cut
			bestOverlap = overlapScore
			bestNovelty = noveltyScore
			bestCombined = combinedScore
			continue
		}
		if combinedScore > secondBestCombined {
			secondBestCombined = combinedScore
		}
	}

	if bestCut == -1 {
		diagnostic.Strategy = "v2_fallback"
		diagnostic.FallbackUsed = true
		return left + "\n\n" + right, unresolvedWarning(boundaryIndex), diagnostic
	}

	diagnostic.Strategy = "v2_trigram"
	diagnostic.ChosenCutRune = bestCut
	diagnostic.OverlapScore = bestOverlap
	diagnostic.NoveltyScore = bestNovelty
	diagnostic.CombinedScore = bestCombined
	if secondBestCombined >= 0 {
		diagnostic.ScoreGap = math.Max(0, bestCombined-secondBestCombined)
	}
	return left + string(rightRunes[bestCut:]), nil, diagnostic
}

func resolveContiguousOverlap(left, right string) (string, int, string, bool) {
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
	for i := minContiguousOverlapRunes; i <= maxRunes; i++ {
		if string(suffixRunes[len(suffixRunes)-i:]) == string(prefixRunes[:i]) {
			best = i
		}
	}
	if best > 0 {
		return left + string(rightRunes[best:]), best, "legacy_exact", true
	}
	for i := minContiguousOverlapRunes; i <= maxRunes; i++ {
		if normalize(string(suffixRunes[len(suffixRunes)-i:])) == normalize(string(prefixRunes[:i])) {
			best = i
		}
	}
	if best > 0 {
		return left + string(rightRunes[best:]), best, "legacy_normalized", true
	}
	return "", 0, "", false
}

func candidateCutPoints(rightRunes []rune, start, end int) []int {
	if len(rightRunes) == 0 {
		return nil
	}
	start = clampInt(start, 0, len(rightRunes))
	end = clampInt(end, 0, len(rightRunes))
	if end <= start {
		return []int{end}
	}

	candidates := collectBoundaryCandidates(rightRunes, start, end, "。！？\n、")
	if len(candidates) == 0 {
		for cut := start; cut <= end; cut += minCandidateStepRunes {
			candidates = append(candidates, cut)
		}
		if candidates[len(candidates)-1] != end {
			candidates = append(candidates, end)
		}
	}
	candidates = append(candidates, start, end)
	if len(candidates) == 0 {
		return nil
	}
	slices.Sort(candidates)
	return slices.Compact(candidates)
}

func collectBoundaryCandidates(rightRunes []rune, start, end int, boundaryChars string) []int {
	out := make([]int, 0)
	for i := start; i < end && i < len(rightRunes); i++ {
		if strings.ContainsRune(boundaryChars, rightRunes[i]) {
			out = append(out, i+1)
		}
	}
	return out
}

func trigramJaccard(a, b string) float64 {
	aRunes := []rune(a)
	bRunes := []rune(b)
	if len(aRunes) < 3 || len(bRunes) < 3 {
		return 0
	}
	setA := trigramSet(aRunes)
	setB := trigramSet(bRunes)
	intersection := 0
	for tri := range setA {
		if _, ok := setB[tri]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func trigramSet(runes []rune) map[string]struct{} {
	out := make(map[string]struct{}, max(0, len(runes)-2))
	for i := 0; i+3 <= len(runes); i++ {
		out[string(runes[i:i+3])] = struct{}{}
	}
	return out
}

func clampInt(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}
