package openai

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestIntegrationTranscribeShortFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}

	fixture := filepath.Join("..", "..", "..", "testdata", "fixtures", "short-ja.m4a")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}

	provider := New(apiKey)
	ctx := context.Background()

	t.Run("gpt-4o-transcribe", func(t *testing.T) {
		resp, err := provider.Transcribe(ctx, domain.ProviderRequest{
			Spec: domain.JobSpec{
				Model:    "gpt-4o-transcribe",
				Language: "ja",
			},
			FilePath:       fixture,
			ResponseFormat: "json",
		})
		if err != nil {
			t.Fatalf("Transcribe() error = %v", err)
		}
		if resp.Transcript.Text == "" {
			t.Fatal("expected non-empty transcript text")
		}
		if resp.Transcript.ModelUsed != "gpt-4o-transcribe" {
			t.Fatalf("unexpected model used: %q", resp.Transcript.ModelUsed)
		}
	})

	t.Run("whisper-1", func(t *testing.T) {
		resp, err := provider.Transcribe(ctx, domain.ProviderRequest{
			Spec: domain.JobSpec{
				Model:    "whisper-1",
				Language: "ja",
			},
			FilePath:       fixture,
			ResponseFormat: "verbose_json",
		})
		if err != nil {
			t.Fatalf("Transcribe() error = %v", err)
		}
		if resp.Transcript.Text == "" {
			t.Fatal("expected non-empty transcript text")
		}
		if len(resp.Transcript.Segments) == 0 {
			t.Fatal("expected timestamp segments from whisper verbose_json")
		}
	})
}
