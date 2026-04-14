package domain

import "fmt"

type ModelProfile struct {
	Model                    string
	SupportsPrompt           bool
	SupportsLogprobs         bool
	SupportsTimestampOutput  bool
	SupportsDiarization      bool
	SupportsServerChunking   bool
	UseOverlapMergeV2        bool
	ParallelChunking         bool
	PromptCarryover          bool
	ChunkTargetSec           float64
	ChunkOverlapSec          float64
	SupportedResponseFormats map[string]bool
}

var transcriptionModelProfiles = map[string]ModelProfile{
	"gpt-4o-transcribe": {
		Model:                   "gpt-4o-transcribe",
		SupportsPrompt:          true,
		SupportsLogprobs:        true,
		SupportsTimestampOutput: false,
		SupportsDiarization:     false,
		SupportsServerChunking:  true,
		UseOverlapMergeV2:       true,
		ParallelChunking:        false,
		PromptCarryover:         true,
		ChunkTargetSec:          300,
		ChunkOverlapSec:         30,
		SupportedResponseFormats: map[string]bool{
			"json": true,
		},
	},
	"gpt-4o-mini-transcribe": {
		Model:                   "gpt-4o-mini-transcribe",
		SupportsPrompt:          true,
		SupportsLogprobs:        true,
		SupportsTimestampOutput: false,
		SupportsDiarization:     false,
		SupportsServerChunking:  true,
		UseOverlapMergeV2:       true,
		ParallelChunking:        false,
		PromptCarryover:         true,
		ChunkTargetSec:          240,
		ChunkOverlapSec:         30,
		SupportedResponseFormats: map[string]bool{
			"json": true,
		},
	},
	"whisper-1": {
		Model:                   "whisper-1",
		SupportsPrompt:          true,
		SupportsLogprobs:        false,
		SupportsTimestampOutput: true,
		SupportsDiarization:     false,
		SupportsServerChunking:  false,
		UseOverlapMergeV2:       false,
		ParallelChunking:        true,
		PromptCarryover:         false,
		ChunkTargetSec:          25,
		ChunkOverlapSec:         5,
		SupportedResponseFormats: map[string]bool{
			"json":         true,
			"text":         true,
			"srt":          true,
			"verbose_json": true,
			"vtt":          true,
		},
	},
	"whisper-1-ts": {
		Model:                   "whisper-1-ts",
		SupportsPrompt:          true,
		SupportsLogprobs:        false,
		SupportsTimestampOutput: true,
		SupportsDiarization:     false,
		SupportsServerChunking:  false,
		UseOverlapMergeV2:       false,
		ParallelChunking:        true,
		PromptCarryover:         false,
		ChunkTargetSec:          25,
		ChunkOverlapSec:         5,
		SupportedResponseFormats: map[string]bool{
			"json":         true,
			"text":         true,
			"srt":          true,
			"verbose_json": true,
			"vtt":          true,
		},
	},
	"gpt-4o-transcribe-diarize": {
		Model:                   "gpt-4o-transcribe-diarize",
		SupportsPrompt:          false,
		SupportsLogprobs:        false,
		SupportsTimestampOutput: true,
		SupportsDiarization:     true,
		SupportsServerChunking:  true,
		UseOverlapMergeV2:       false,
		ParallelChunking:        false,
		PromptCarryover:         false,
		ChunkTargetSec:          300,
		ChunkOverlapSec:         30,
		SupportedResponseFormats: map[string]bool{
			"json":          true,
			"text":          true,
			"diarized_json": true,
		},
	},
}

type ProviderRequest struct {
	Spec           JobSpec
	FilePath       string
	ResponseFormat string
}

type ProviderResponse struct {
	Transcript Transcript
	RawJSON    string
}

func (s JobSpec) IsDiarize() bool {
	return s.Model == "gpt-4o-transcribe-diarize"
}

func KnownTranscriptionModel(model string) bool {
	_, ok := transcriptionModelProfiles[model]
	return ok
}

func ModelProfileFor(model string) (ModelProfile, bool) {
	profile, ok := transcriptionModelProfiles[model]
	return profile, ok
}

func SupportsTimestampOutput(model string) bool {
	profile, ok := ModelProfileFor(model)
	return ok && profile.SupportsTimestampOutput
}

func SupportsDiarization(model string) bool {
	profile, ok := ModelProfileFor(model)
	return ok && profile.SupportsDiarization
}

func SupportsPrompt(model string) bool {
	profile, ok := ModelProfileFor(model)
	return ok && profile.SupportsPrompt
}

func SupportsLogprobs(model string) bool {
	profile, ok := ModelProfileFor(model)
	return ok && profile.SupportsLogprobs
}

func SupportsResponseFormat(model, responseFormat string) bool {
	profile, ok := ModelProfileFor(model)
	return ok && profile.SupportedResponseFormats[responseFormat]
}

func ValidateModel(model string) error {
	if KnownTranscriptionModel(model) {
		return nil
	}
	return NewError("model_not_supported", fmt.Sprintf("unsupported model: %s", model), ExitArgs, nil)
}

func ValidateSubtitleFormat(model string, format OutputFormat) error {
	if format != FormatSRT && format != FormatVTT {
		return nil
	}
	if SupportsTimestampOutput(model) {
		return nil
	}
	return NewError("format_not_supported", "subtitle formats require a timestamp-capable model", ExitArgs, nil)
}

func ValidatePrompt(model, prompt string) error {
	if prompt == "" || SupportsPrompt(model) {
		return nil
	}
	return NewError("diarize_prompt_unsupported", "prompt is not supported with diarize", ExitArgs, nil)
}

func ValidateLogprobs(model string, enabled bool) error {
	if !enabled || SupportsLogprobs(model) {
		return nil
	}
	return NewError("logprobs_not_supported", "--logprobs is only supported with gpt-4o-transcribe and gpt-4o-mini-transcribe", ExitArgs, nil)
}
