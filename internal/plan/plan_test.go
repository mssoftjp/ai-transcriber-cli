package plan

import (
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestPlannerAutoFallsBackToClientForLargeFiles(t *testing.T) {
	planner := NewPlanner()
	spec := domain.JobSpec{
		InputPath:     "/tmp/input.mp3",
		Format:        domain.FormatMD,
		Model:         "gpt-4o-transcribe",
		Language:      "ja",
		ChunkingMode:  domain.ChunkingAuto,
		WriteManifest: true,
	}
	input := domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   30 * 1024 * 1024,
		DurationSec: 900,
		HasAudio:    true,
	}
	plan, err := planner.Build(spec, input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.ChunkingMode != domain.ChunkingClient {
		t.Fatalf("expected client chunking, got %s", plan.ChunkingMode)
	}
	if len(plan.Chunks) == 0 {
		t.Fatalf("expected generated chunks for large file")
	}
}

func TestPlannerUsesSingleRequestForWhisperAutoWhenInputFits(t *testing.T) {
	planner := NewPlanner()
	spec := domain.JobSpec{
		InputPath:    "/tmp/input.mp3",
		Format:       domain.FormatJSON,
		Model:        "whisper-1",
		ChunkingMode: domain.ChunkingAuto,
	}
	input := domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   2 * 1024 * 1024,
		DurationSec: 12,
		HasAudio:    true,
	}

	plan, err := planner.Build(spec, input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.ChunkingMode != domain.ChunkingOff {
		t.Fatalf("expected single-request chunking off, got %s", plan.ChunkingMode)
	}
}

func TestPlannerRejectsUnsupportedExtensionAsInputError(t *testing.T) {
	planner := NewPlanner()
	_, err := planner.Build(domain.JobSpec{
		InputPath: "/tmp/input.txt",
		Format:    domain.FormatMD,
		Model:     "gpt-4o-transcribe",
	}, domain.InputInfo{
		Path:        "/tmp/input.txt",
		SizeBytes:   128,
		DurationSec: 10,
		HasAudio:    false,
	})
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if domain.ExitCode(err) != domain.ExitInput {
		t.Fatalf("expected ExitInput, got %d (%v)", domain.ExitCode(err), err)
	}
}

func TestPlannerRejectsUnsupportedModelAsArgsError(t *testing.T) {
	planner := NewPlanner()
	_, err := planner.Build(domain.JobSpec{
		InputPath: "/tmp/input.mp3",
		Format:    domain.FormatMD,
		Model:     "future-transcribe",
	}, domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   128,
		DurationSec: 10,
		HasAudio:    true,
	})
	if err == nil {
		t.Fatal("expected error for unsupported model")
	}
	if domain.ExitCode(err) != domain.ExitArgs {
		t.Fatalf("expected ExitArgs, got %d (%v)", domain.ExitCode(err), err)
	}
}

func TestPlannerRequiresChunkingForLongDiarizeInput(t *testing.T) {
	planner := NewPlanner()
	_, err := planner.Build(domain.JobSpec{
		InputPath:    "/tmp/input.mp3",
		Format:       domain.FormatJSON,
		Model:        "gpt-4o-transcribe-diarize",
		ChunkingMode: domain.ChunkingOff,
	}, domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   2 * 1024 * 1024,
		DurationSec: 45,
		HasAudio:    true,
	})
	if err == nil {
		t.Fatal("expected error for long diarize input without chunking")
	}
	if domain.ErrorCode(err) != "diarize_chunking_required" {
		t.Fatalf("unexpected error code: %s", domain.ErrorCode(err))
	}
}

func TestPlannerRejectsLongDiarizeChunkStitchingWithoutExperimentalFlag(t *testing.T) {
	planner := NewPlanner()
	_, err := planner.Build(domain.JobSpec{
		InputPath:    "/tmp/input.mp3",
		Format:       domain.FormatJSON,
		Model:        "gpt-4o-transcribe-diarize",
		ChunkingMode: domain.ChunkingAuto,
	}, domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   30 * 1024 * 1024,
		DurationSec: 180,
		HasAudio:    true,
	})
	if err == nil {
		t.Fatal("expected error for long diarize stitching without experimental flag")
	}
	if domain.ErrorCode(err) != "diarize_stitching_experimental" {
		t.Fatalf("unexpected error code: %s", domain.ErrorCode(err))
	}
}

func TestPlannerAllowsLongDiarizeChunkStitchingWithExperimentalFlag(t *testing.T) {
	planner := NewPlanner()
	execPlan, err := planner.Build(domain.JobSpec{
		InputPath:                         "/tmp/input.mp3",
		Format:                            domain.FormatJSON,
		Model:                             "gpt-4o-transcribe-diarize",
		ChunkingMode:                      domain.ChunkingAuto,
		AllowExperimentalDiarizeStitching: true,
	}, domain.InputInfo{
		Path:        "/tmp/input.mp3",
		SizeBytes:   30 * 1024 * 1024,
		DurationSec: 180,
		HasAudio:    true,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if execPlan.ChunkingMode != domain.ChunkingClient {
		t.Fatalf("expected client chunking, got %s", execPlan.ChunkingMode)
	}
}

func TestPlannerRequiresFFmpegForProviderIncompatibleAudioExtension(t *testing.T) {
	planner := NewPlanner()
	execPlan, err := planner.Build(domain.JobSpec{
		InputPath: "/tmp/input.aac",
		Format:    domain.FormatMD,
		Model:     "gpt-4o-transcribe",
	}, domain.InputInfo{
		Path:        "/tmp/input.aac",
		SizeBytes:   1024,
		DurationSec: 10,
		HasAudio:    true,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !execPlan.FFmpegRequired {
		t.Fatal("expected ffmpeg to be required for provider-incompatible extension")
	}
}
