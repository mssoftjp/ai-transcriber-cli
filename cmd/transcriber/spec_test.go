package main

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
)

func TestBuildSpecBoolFlagsOverrideConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Transcription.Logprobs = true
	cfg.Dictionary.Enabled = true
	cfg.Postprocess.Enabled = true
	cfg.Paths.KeepWorkdir = true
	cfg.Output.Overwrite = true
	cfg.Output.WriteManifest = false

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("logprobs", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("dictionary-enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("postprocess", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("keep-workdir", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("overwrite", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("write-manifest", "true"); err != nil {
		t.Fatal(err)
	}

	spec, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err != nil {
		t.Fatalf("buildSpec() error = %v", err)
	}
	if spec.Logprobs || spec.DictionaryEnabled || spec.Postprocess || spec.KeepWorkdir || spec.Overwrite {
		t.Fatalf("expected explicit false CLI flags to override config true values: %#v", spec)
	}
	if !spec.WriteManifest {
		t.Fatal("expected explicit true CLI flag to override config false value")
	}
}

func TestBuildSpecRejectsUnsupportedLogprobsModel(t *testing.T) {
	cfg := config.Default()
	cfg.Transcription.Model = "whisper-1"

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("logprobs", "true"); err != nil {
		t.Fatal(err)
	}

	_, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err == nil {
		t.Fatal("expected error for unsupported logprobs model")
	}
	if code := domain.ErrorCode(err); code != "logprobs_not_supported" {
		t.Fatalf("unexpected error code: %s", code)
	}
}

func TestBuildSpecCarriesConfiguredAPIKeyEnv(t *testing.T) {
	cfg := config.Default()
	cfg.API.KeyEnv = "MY_OPENAI_KEY"

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)

	spec, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err != nil {
		t.Fatalf("buildSpec() error = %v", err)
	}
	if spec.APIKeyEnv != "MY_OPENAI_KEY" {
		t.Fatalf("APIKeyEnv = %q, want MY_OPENAI_KEY", spec.APIKeyEnv)
	}
}

func TestBuildSpecNormalizesDiarizeToModel(t *testing.T) {
	cfg := config.Default()

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("diarize", "true"); err != nil {
		t.Fatal(err)
	}

	spec, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err != nil {
		t.Fatalf("buildSpec() error = %v", err)
	}
	if !spec.IsDiarize() {
		t.Fatal("expected diarize flag to normalize into diarize model")
	}
	if spec.Model != "gpt-4o-transcribe-diarize" {
		t.Fatalf("Model = %q, want gpt-4o-transcribe-diarize", spec.Model)
	}
}

func TestBuildSpecResolvesSpeakerRefPaths(t *testing.T) {
	cfg := config.Default()

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("speaker-ref", "agent=./fixtures/agent.m4a"); err != nil {
		t.Fatal(err)
	}

	spec, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err != nil {
		t.Fatalf("buildSpec() error = %v", err)
	}
	if len(spec.SpeakerRefs) != 1 {
		t.Fatalf("expected one speaker ref, got %#v", spec.SpeakerRefs)
	}
	if !filepath.IsAbs(spec.SpeakerRefs[0].Path) {
		t.Fatalf("expected absolute speaker ref path, got %q", spec.SpeakerRefs[0].Path)
	}
}

func TestBuildSpecRejectsMalformedSpeakerRef(t *testing.T) {
	cfg := config.Default()

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("speaker-ref", "broken"); err != nil {
		t.Fatal(err)
	}

	_, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err == nil {
		t.Fatal("expected malformed speaker ref error")
	}
	if code := domain.ErrorCode(err); code != "speaker_ref_invalid" {
		t.Fatalf("unexpected error code: %s", code)
	}
}

func TestBuildSpecCarriesChunkOverrides(t *testing.T) {
	cfg := config.Default()

	cmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(cmd)
	if err := cmd.Flags().Set("chunk-target-sec", "150"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("chunk-overlap-sec", "30"); err != nil {
		t.Fatal(err)
	}

	spec, err := buildSpec(cfg, cmd, "/tmp/input.mp3")
	if err != nil {
		t.Fatalf("buildSpec() error = %v", err)
	}
	if spec.ChunkTargetSecOverride == nil || *spec.ChunkTargetSecOverride != 150 {
		t.Fatalf("expected chunk target override, got %#v", spec.ChunkTargetSecOverride)
	}
	if spec.ChunkOverlapSecOverride == nil || *spec.ChunkOverlapSecOverride != 30 {
		t.Fatalf("expected chunk overlap override, got %#v", spec.ChunkOverlapSecOverride)
	}
}
