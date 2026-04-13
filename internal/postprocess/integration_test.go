package postprocess

import (
	"context"
	"os"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestIntegrationApplyAI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}

	svc := NewService(apiKey)
	transcript, err := svc.Apply(context.Background(), domain.Transcript{
		Text: "  本日の 会議 では、 ai transcriber cli の 仕様 を さい確認 しました。  ",
	}, true, "gpt-4o-mini", "Correct obvious ASR mistakes and normalize spacing, but keep the same language and meaning.")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if transcript.Text == "" {
		t.Fatal("expected non-empty postprocessed transcript")
	}
}
