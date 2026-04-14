package plan

import (
	"fmt"
	"path/filepath"
	"strings"

	"ai-transcriber-cli/internal/domain"
)

type ExecutionPlan struct {
	Input                     domain.InputInfo    `json:"input"`
	FFmpegRequired            bool                `json:"ffmpeg_required"`
	NormalizationRequired     bool                `json:"normalization_required"`
	SingleRequestPossible     bool                `json:"single_request_possible"`
	ChunkingMode              domain.ChunkingMode `json:"chunking_mode"`
	ResponseFormat            string              `json:"response_format"`
	TimestampCapable          bool                `json:"timestamp_capable"`
	DiarizeCapable            bool                `json:"diarize_capable"`
	EstimatedIntermediateSize int64               `json:"estimated_intermediate_size"`
	Artifacts                 []domain.Artifact   `json:"artifacts,omitempty"`
	Chunks                    []domain.ChunkPlan  `json:"chunks,omitempty"`
}

type Planner struct{}

func NewPlanner() *Planner { return &Planner{} }

func (p *Planner) Build(spec domain.JobSpec, input domain.InputInfo) (ExecutionPlan, error) {
	if !domain.SupportedInputExtension(spec.InputPath) {
		return ExecutionPlan{}, domain.NewError("unsupported_input_extension", fmt.Sprintf("unsupported input extension: %s", filepath.Ext(spec.InputPath)), domain.ExitInput, nil)
	}
	if err := domain.ValidateModel(spec.Model); err != nil {
		return ExecutionPlan{}, err
	}
	profile, _ := domain.ModelProfileFor(spec.Model)
	if spec.ChunkTargetSecOverride != nil {
		if *spec.ChunkTargetSecOverride <= 0 {
			return ExecutionPlan{}, domain.NewError("chunk_target_invalid", "--chunk-target-sec must be greater than zero", domain.ExitArgs, nil)
		}
		profile.ChunkTargetSec = *spec.ChunkTargetSecOverride
	}
	if spec.ChunkOverlapSecOverride != nil {
		if *spec.ChunkOverlapSecOverride < 0 {
			return ExecutionPlan{}, domain.NewError("chunk_overlap_invalid", "--chunk-overlap-sec must be zero or greater", domain.ExitArgs, nil)
		}
		profile.ChunkOverlapSec = *spec.ChunkOverlapSecOverride
	}
	if profile.ChunkTargetSec > 0 && profile.ChunkOverlapSec >= profile.ChunkTargetSec {
		return ExecutionPlan{}, domain.NewError("chunk_overlap_invalid", "--chunk-overlap-sec must be smaller than --chunk-target-sec", domain.ExitArgs, nil)
	}
	effectiveInput := applyInputWindow(spec, input)
	plan := ExecutionPlan{
		Input:                     effectiveInput,
		FFmpegRequired:            input.IsVideo || spec.StartSec != nil || spec.EndSec != nil || spec.ChunkingMode == domain.ChunkingClient || spec.VADMode == domain.VADLocal || !domain.ProviderCompatibleAudioExtension(spec.InputPath),
		NormalizationRequired:     spec.ChunkingMode == domain.ChunkingClient || spec.VADMode == domain.VADLocal,
		SingleRequestPossible:     input.SizeBytes > 0 && input.SizeBytes <= domain.ProviderUploadLimitBytes,
		TimestampCapable:          domain.SupportsTimestampOutput(spec.Model),
		DiarizeCapable:            spec.IsDiarize(),
		EstimatedIntermediateSize: estimateIntermediateSize(effectiveInput),
	}
	plan.ChunkingMode = resolveChunkingMode(spec, plan)
	if plan.DiarizeCapable && effectiveInput.DurationSec > 30 && plan.ChunkingMode == domain.ChunkingOff {
		return ExecutionPlan{}, domain.NewError("diarize_chunking_required", "diarize inputs longer than 30 seconds require chunking", domain.ExitArgs, nil)
	}
	if plan.DiarizeCapable && effectiveInput.DurationSec > 30 && plan.ChunkingMode == domain.ChunkingClient && !spec.AllowExperimentalDiarizeStitching {
		return ExecutionPlan{}, domain.NewError("diarize_stitching_experimental", "long-form diarize chunk stitching requires --allow-experimental-diarize-stitching", domain.ExitArgs, nil)
	}
	if plan.ChunkingMode == domain.ChunkingOff && !plan.SingleRequestPossible {
		return ExecutionPlan{}, domain.NewError("chunking_off_too_large", "chunking_mode=off cannot be used for inputs larger than 25MB", domain.ExitArgs, nil)
	}
	plan.ResponseFormat = responseFormatFor(spec, plan)
	plan.Artifacts = BuildArtifacts(spec)
	if plan.ChunkingMode == domain.ChunkingClient && effectiveInput.DurationSec > 0 {
		plan.Chunks = buildChunks(profile, effectiveInput.DurationSec)
	}
	return plan, nil
}

func applyInputWindow(spec domain.JobSpec, input domain.InputInfo) domain.InputInfo {
	if input.DurationSec <= 0 {
		return input
	}
	start := 0.0
	if spec.StartSec != nil && *spec.StartSec > 0 {
		start = *spec.StartSec
	}
	end := input.DurationSec
	if spec.EndSec != nil && *spec.EndSec >= 0 && *spec.EndSec < end {
		end = *spec.EndSec
	}
	if end < start {
		end = start
	}
	input.DurationSec = end - start
	return input
}

func resolveChunkingMode(spec domain.JobSpec, plan ExecutionPlan) domain.ChunkingMode {
	if spec.ChunkingMode != domain.ChunkingAuto {
		return spec.ChunkingMode
	}
	if !plan.SingleRequestPossible {
		return domain.ChunkingClient
	}
	profile, ok := domain.ModelProfileFor(spec.Model)
	if !ok || !profile.SupportsServerChunking {
		// Models without provider-side chunking should use a plain single request
		// when the file already fits within the API size limit.
		return domain.ChunkingOff
	}
	return domain.ChunkingServerAuto
}

func responseFormatFor(spec domain.JobSpec, plan ExecutionPlan) string {
	switch spec.Format {
	case domain.FormatSRT, domain.FormatVTT:
		if spec.IsDiarize() {
			return "diarized_json"
		}
		return "verbose_json"
	default:
		if spec.IsDiarize() {
			return "diarized_json"
		}
		if plan.TimestampCapable {
			return "verbose_json"
		}
		return "json"
	}
}

func estimateIntermediateSize(input domain.InputInfo) int64 {
	if input.DurationSec <= 0 {
		return input.SizeBytes
	}
	return int64(input.DurationSec * domain.IntermediateAudioBytesPerSec)
}

func BuildArtifacts(spec domain.JobSpec) []domain.Artifact {
	if spec.Stdout || spec.DryRun {
		return nil
	}
	path := spec.OutputPath
	if path == "" {
		base := strings.TrimSuffix(filepath.Base(spec.InputPath), filepath.Ext(spec.InputPath))
		filename := fmt.Sprintf("%s.transcript.%s", base, spec.Format)
		if spec.OutputDir != "" {
			path = filepath.Join(spec.OutputDir, filename)
		} else {
			path = filepath.Join(filepath.Dir(spec.InputPath), filename)
		}
	}
	artifacts := []domain.Artifact{{Path: path, Format: spec.Format, Partial: false}}
	if spec.WriteManifest {
		artifacts[0].ManifestPath = manifestPath(path)
	}
	return artifacts
}

func PartialArtifact(spec domain.JobSpec) domain.Artifact {
	path := spec.OutputPath
	if path == "" {
		base := strings.TrimSuffix(filepath.Base(spec.InputPath), filepath.Ext(spec.InputPath))
		filename := fmt.Sprintf("%s.transcript.partial.%s", base, spec.Format)
		if spec.OutputDir != "" {
			path = filepath.Join(spec.OutputDir, filename)
		} else {
			path = filepath.Join(filepath.Dir(spec.InputPath), filename)
		}
	} else {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".partial" + filepath.Ext(path)
	}
	return domain.Artifact{Path: path, Format: spec.Format, Partial: true, ManifestPath: manifestPath(path)}
}

func manifestPath(outputPath string) string {
	base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	base = strings.TrimSuffix(base, ".partial")
	return base + ".manifest.json"
}

func buildChunks(profile domain.ModelProfile, durationSec float64) []domain.ChunkPlan {
	target, overlap := profile.ChunkTargetSec, profile.ChunkOverlapSec
	var chunks []domain.ChunkPlan
	start := 0.0
	index := 0
	for start < durationSec {
		end := start + target
		if end > durationSec {
			end = durationSec
		}
		chunks = append(chunks, domain.ChunkPlan{
			Index:           index,
			StartSec:        start,
			EndSec:          end,
			OverlapSec:      overlap,
			PromptCarryover: profile.PromptCarryover,
		})
		if end == durationSec {
			break
		}
		start = end - overlap
		index++
	}
	return chunks
}
