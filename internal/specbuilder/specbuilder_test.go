package specbuilder

import (
	"path/filepath"
	"testing"

	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
)

func TestBuildUsesDefaultsAndEnvFallbacks(t *testing.T) {
	cfg := config.Default()
	cfg.Paths.FFmpeg = "ffmpeg-config"
	cfg.Paths.FFprobe = "ffprobe-config"
	cfg.Transcription.Format = domain.FormatJSON
	cfg.Output.WriteManifest = false
	cwd := t.TempDir()

	spec, err := Build(Input{InputPath: "input.m4a"}, Defaults{
		Config: cfg,
		CWD:    cwd,
		Env: map[string]string{
			"TRANSCRIBER_FFMPEG":  "ffmpeg-env",
			"TRANSCRIBER_FFPROBE": "ffprobe-env",
		},
		JobID: "job-test",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if spec.JobID != "job-test" {
		t.Fatalf("JobID = %q, want job-test", spec.JobID)
	}
	if spec.InputPath != filepath.Join(cwd, "input.m4a") {
		t.Fatalf("InputPath = %q", spec.InputPath)
	}
	if spec.FFmpegPath != "ffmpeg-env" || spec.FFprobePath != "ffprobe-env" {
		t.Fatalf("paths = %q/%q, want env fallbacks", spec.FFmpegPath, spec.FFprobePath)
	}
	if spec.Format != domain.FormatJSON {
		t.Fatalf("Format = %q, want json", spec.Format)
	}
	if spec.WriteManifest {
		t.Fatal("expected config false write manifest default")
	}
}

func TestBuildExplicitBoolOverridesConfigFalse(t *testing.T) {
	cfg := config.Default()
	cfg.Output.WriteManifest = false
	writeManifest := true
	logprobs := true

	spec, err := Build(Input{
		InputPath:      "/tmp/input.m4a",
		Model:          "gpt-4o-transcribe",
		WriteManifest:  &writeManifest,
		Logprobs:       &logprobs,
		ChunkTargetSec: ptrFloat(600),
	}, Defaults{Config: cfg, JobID: "job-test"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !spec.WriteManifest || !spec.Logprobs {
		t.Fatalf("expected explicit bools to win: %#v", spec)
	}
	if spec.ChunkTargetSecOverride == nil || *spec.ChunkTargetSecOverride != 600 {
		t.Fatalf("ChunkTargetSecOverride = %#v, want 600", spec.ChunkTargetSecOverride)
	}
}

func TestBuildRejectsInvalidSubtitleModel(t *testing.T) {
	_, err := Build(Input{
		InputPath: "/tmp/input.m4a",
		Model:     "gpt-4o-transcribe",
		Format:    "srt",
	}, Defaults{Config: config.Default(), JobID: "job-test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := domain.ErrorCode(err); code != "format_not_supported" {
		t.Fatalf("error code = %q, want format_not_supported", code)
	}
}

func TestBuildRejectsTimeRange(t *testing.T) {
	_, err := Build(Input{
		InputPath: "/tmp/input.m4a",
		Start:     "00:02:00",
		End:       "60",
	}, Defaults{Config: config.Default(), JobID: "job-test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := domain.ErrorCode(err); code != "time_range_invalid" {
		t.Fatalf("error code = %q, want time_range_invalid", code)
	}
}

func TestBuildResolvesSpeakerRefs(t *testing.T) {
	cwd := t.TempDir()

	spec, err := Build(Input{
		InputPath:   "/tmp/input.m4a",
		SpeakerRefs: []string{"host=refs/host.m4a"},
	}, Defaults{Config: config.Default(), CWD: cwd, JobID: "job-test"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := spec.SpeakerRefs[0].Path; got != filepath.Join(cwd, "refs/host.m4a") {
		t.Fatalf("speaker ref path = %q", got)
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
