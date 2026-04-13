package vad

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"ai-transcriber-cli/internal/domain"
)

type Optimizer interface {
	Optimize(ctx context.Context, spec domain.JobSpec, chunks []domain.ChunkPlan) ([]domain.ChunkPlan, []domain.Warning, error)
}

type Service struct{}

func NewService() *Service { return &Service{} }

type silenceSpan struct {
	StartSec float64
	EndSec   float64
}

const (
	silenceNoiseDB  = "-30dB"
	silenceMinDur   = 0.35
	fallbackCode    = "local_vad_fallback"
	fallbackMessage = "local VAD boundary optimization was unavailable; falling back to time-based chunking"
)

func (s *Service) Optimize(ctx context.Context, spec domain.JobSpec, chunks []domain.ChunkPlan) ([]domain.ChunkPlan, []domain.Warning, error) {
	if spec.VADMode != domain.VADLocal || len(chunks) < 2 {
		return chunks, nil, nil
	}
	ffmpeg := spec.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	silences, err := detectSilences(ctx, ffmpeg, spec)
	if err != nil {
		return chunks, []domain.Warning{{
			Code:    fallbackCode,
			Message: fallbackMessage,
		}}, nil
	}
	if len(silences) == 0 {
		return chunks, nil, nil
	}
	// The optimizer only nudges boundaries toward nearby silence; it never changes
	// chunk ordering or removes overlap guarantees from the planner.
	return optimizeChunkBoundaries(chunks, silences), nil, nil
}

var (
	silenceStartRE = regexp.MustCompile(`silence_start:\s*([0-9.]+)`)
	silenceEndRE   = regexp.MustCompile(`silence_end:\s*([0-9.]+)`)
)

func detectSilences(ctx context.Context, ffmpegPath string, spec domain.JobSpec) ([]silenceSpan, error) {
	args := []string{"-hide_banner", "-nostats", "-v", "info"}
	if spec.StartSec != nil {
		args = append(args, "-ss", fmt.Sprintf("%.3f", *spec.StartSec))
	}
	args = append(args, "-i", spec.InputPath)
	if spec.StartSec != nil && spec.EndSec != nil {
		args = append(args, "-t", fmt.Sprintf("%.3f", *spec.EndSec-*spec.StartSec))
	} else if spec.EndSec != nil {
		args = append(args, "-t", fmt.Sprintf("%.3f", *spec.EndSec))
	}
	args = append(args, "-af", fmt.Sprintf("silencedetect=noise=%s:d=%.2f", silenceNoiseDB, silenceMinDur), "-f", "null", "-")
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return parseSilenceDetect(string(output)), nil
}

func parseSilenceDetect(output string) []silenceSpan {
	lines := strings.Split(output, "\n")
	spans := make([]silenceSpan, 0)
	currentStart := -1.0
	for _, line := range lines {
		if matches := silenceStartRE.FindStringSubmatch(line); len(matches) == 2 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				currentStart = value
			}
			continue
		}
		if matches := silenceEndRE.FindStringSubmatch(line); len(matches) == 2 && currentStart >= 0 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err == nil && value > currentStart {
				spans = append(spans, silenceSpan{StartSec: currentStart, EndSec: value})
			}
			currentStart = -1
		}
	}
	return spans
}

func optimizeChunkBoundaries(chunks []domain.ChunkPlan, silences []silenceSpan) []domain.ChunkPlan {
	out := append([]domain.ChunkPlan(nil), chunks...)
	for i := 0; i < len(out)-1; i++ {
		target := out[i].EndSec
		searchWindow := clampFloat(maxFloat(out[i].OverlapSec*2, 2), 2, 15)
		boundary, ok := nearestSilenceBoundary(target, searchWindow, silences)
		if !ok {
			continue
		}
		minEnd := out[i].StartSec + 1
		maxEnd := out[i+1].EndSec - 0.5
		if boundary <= minEnd || boundary >= maxEnd {
			continue
		}
		out[i].EndSec = boundary
		nextStart := boundary - out[i+1].OverlapSec
		if nextStart < 0 {
			nextStart = 0
		}
		if nextStart >= out[i+1].EndSec {
			continue
		}
		out[i+1].StartSec = nextStart
	}
	return out
}

func nearestSilenceBoundary(target, window float64, silences []silenceSpan) (float64, bool) {
	best := 0.0
	bestDistance := 0.0
	found := false
	for _, silence := range silences {
		boundary := (silence.StartSec + silence.EndSec) / 2
		distance := absFloat(boundary - target)
		if distance > window {
			continue
		}
		if !found || distance < bestDistance {
			best = boundary
			bestDistance = distance
			found = true
		}
	}
	return best, found
}

func clampFloat(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
