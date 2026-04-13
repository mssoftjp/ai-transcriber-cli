package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-transcriber-cli/internal/domain"
)

type Service struct{}

func NewService() *Service { return &Service{} }

type ffprobeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

func (s *Service) Probe(ctx context.Context, inputPath, ffprobePath string) (domain.InputInfo, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return domain.InputInfo{}, domain.NewError("input_not_found", "input file is not accessible", domain.ExitInput, err)
	}
	result := domain.InputInfo{
		Path:      inputPath,
		SizeBytes: info.Size(),
		Extension: strings.ToLower(filepath.Ext(inputPath)),
		IsVideo:   domain.IsVideoInput(inputPath),
	}
	if !domain.SupportedInputExtension(inputPath) {
		return result, domain.NewError("unsupported_input", "unsupported input file extension", domain.ExitInput, nil)
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, ffprobePath, "-v", "quiet", "-print_format", "json", "-show_streams", "-show_format", inputPath)
	out, err := cmd.Output()
	if err != nil {
		result.HasAudio = !result.IsVideo
		return result, nil
	}
	var probe ffprobeResult
	if err := json.Unmarshal(out, &probe); err != nil {
		return result, nil
	}
	result.Container = probe.Format.FormatName
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			result.HasAudio = true
			break
		}
	}
	if probe.Format.Duration != "" {
		var duration float64
		_, _ = fmt.Sscanf(probe.Format.Duration, "%f", &duration)
		result.DurationSec = duration
	}
	result.FFmpegNeeded = result.IsVideo
	return result, nil
}

func (s *Service) Chunk(ctx context.Context, spec domain.JobSpec, chunks []domain.ChunkPlan, workdir string) ([]string, error) {
	ffmpeg := spec.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if workdir == "" {
		workdir = os.TempDir()
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	baseOffset := 0.0
	if spec.StartSec != nil {
		baseOffset = *spec.StartSec
	}
	for _, chunk := range chunks {
		chunkPath := filepath.Join(workdir, fmt.Sprintf("chunk-%03d.wav", chunk.Index))
		chunkStart := baseOffset + chunk.StartSec
		duration := chunk.EndSec - chunk.StartSec
		args := []string{"-y", "-ss", fmt.Sprintf("%.3f", chunkStart), "-i", spec.InputPath, "-t", fmt.Sprintf("%.3f", duration), "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", chunkPath}
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, domain.NewError("ffmpeg_chunk_failed", string(output), domain.ExitDecode, err)
		}
		paths = append(paths, chunkPath)
	}
	return paths, nil
}

func (s *Service) Normalize(ctx context.Context, spec domain.JobSpec, workdir string) (string, error) {
	ffmpeg := spec.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if workdir == "" {
		workdir = os.TempDir()
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(workdir, "normalized.wav")
	args := []string{"-y"}
	if spec.StartSec != nil {
		args = append(args, "-ss", fmt.Sprintf("%.3f", *spec.StartSec))
	}
	args = append(args, "-i", spec.InputPath)
	if spec.StartSec != nil && spec.EndSec != nil {
		args = append(args, "-t", fmt.Sprintf("%.3f", *spec.EndSec-*spec.StartSec))
	} else if spec.EndSec != nil {
		args = append(args, "-t", fmt.Sprintf("%.3f", *spec.EndSec))
	}
	args = append(args, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", outPath)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", domain.NewError("ffmpeg_normalize_failed", string(output), domain.ExitDecode, err)
	}
	return outPath, nil
}
