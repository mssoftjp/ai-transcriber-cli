package config

import "testing"

func TestValidateConfigRejectsUnsupportedLogprobsModel(t *testing.T) {
	cfg := Default()
	cfg.Transcription.Model = "whisper-1"
	cfg.Transcription.Logprobs = true

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

func TestValidateConfigAllowsLogprobsFor4oTranscribe(t *testing.T) {
	cfg := Default()
	cfg.Transcription.Model = "gpt-4o-transcribe"
	cfg.Transcription.Logprobs = true

	errs := ValidateConfig(cfg)
	for _, err := range errs {
		if err != nil && err.Error() == "transcription.logprobs is only supported with gpt-4o-transcribe and gpt-4o-mini-transcribe" {
			t.Fatalf("unexpected logprobs validation error: %v", err)
		}
	}
}
