package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"ai-transcriber-cli/internal/dictionary"
	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/events"
	"ai-transcriber-cli/internal/fs"
	"ai-transcriber-cli/internal/logging"
	"ai-transcriber-cli/internal/merge"
	"ai-transcriber-cli/internal/plan"
	"ai-transcriber-cli/internal/render"
)

type MediaService interface {
	Probe(ctx context.Context, inputPath, ffprobePath string) (domain.InputInfo, error)
	Chunk(ctx context.Context, spec domain.JobSpec, chunks []domain.ChunkPlan, workdir string) ([]string, error)
	Normalize(ctx context.Context, spec domain.JobSpec, workdir string) (string, error)
}

type BoundaryOptimizer interface {
	Optimize(ctx context.Context, spec domain.JobSpec, chunks []domain.ChunkPlan) ([]domain.ChunkPlan, []domain.Warning, error)
}

type Planner interface {
	Build(spec domain.JobSpec, input domain.InputInfo) (plan.ExecutionPlan, error)
}

type Provider interface {
	Transcribe(ctx context.Context, req domain.ProviderRequest) (domain.ProviderResponse, error)
	CheckConnectivity(ctx context.Context) error
}

type Postprocessor interface {
	Apply(ctx context.Context, t domain.Transcript, enabled bool, model string, prompt string) (domain.Transcript, error)
}

type Services struct {
	Media       MediaService
	Planner     Planner
	Provider    Provider
	Optimizer   BoundaryOptimizer
	Postprocess Postprocessor
	Logger      *logging.Logger
	Events      events.Writer
}

func New(mediaSvc MediaService, planner Planner, provider Provider, optimizer BoundaryOptimizer, post Postprocessor, logger *logging.Logger, eventWriter events.Writer) *Services {
	return &Services{Media: mediaSvc, Planner: planner, Provider: provider, Optimizer: optimizer, Postprocess: post, Logger: logger, Events: eventWriter}
}

func GenerateJobID() string {
	return fmt.Sprintf("job-%d-%06d", time.Now().UTC().UnixMilli(), rand.Intn(1000000))
}

func (s *Services) Probe(ctx context.Context, spec domain.JobSpec) (domain.ProbeResult, plan.ExecutionPlan, error) {
	input, err := s.Media.Probe(ctx, spec.InputPath, spec.FFprobePath)
	if err != nil {
		return domain.ProbeResult{}, plan.ExecutionPlan{}, err
	}
	execPlan, err := s.Planner.Build(spec, input)
	if err != nil {
		return domain.ProbeResult{}, plan.ExecutionPlan{}, err
	}
	result := domain.ProbeResult{
		ProtocolVersion:           domain.ProtocolVersion,
		Input:                     spec.InputPath,
		Container:                 input.Container,
		HasAudioStream:            input.HasAudio,
		DurationSec:               input.DurationSec,
		FFmpegRequired:            execPlan.FFmpegRequired,
		SingleRequestPossible:     execPlan.SingleRequestPossible,
		PlannedChunkingMode:       execPlan.ChunkingMode,
		DiarizeCapable:            execPlan.DiarizeCapable,
		TimestampCapable:          execPlan.TimestampCapable,
		EstimatedIntermediateSize: execPlan.EstimatedIntermediateSize,
		OutputFormats: []domain.FormatSupport{
			{Format: domain.FormatTXT, Supported: true},
			{Format: domain.FormatMD, Supported: true},
			{Format: domain.FormatJSON, Supported: true},
			{Format: domain.FormatSRT, Supported: execPlan.TimestampCapable, Reason: unsupportedReason(execPlan.TimestampCapable, "timestamp-capable model required")},
			{Format: domain.FormatVTT, Supported: execPlan.TimestampCapable, Reason: unsupportedReason(execPlan.TimestampCapable, "timestamp-capable model required")},
		},
	}
	return result, execPlan, nil
}

func unsupportedReason(supported bool, reason string) string {
	if supported {
		return ""
	}
	return reason
}

func (s *Services) Doctor(ctx context.Context, spec domain.JobSpec, apiKey string) (domain.DoctorResult, error) {
	var checks []domain.DoctorCheck
	checks = append(checks, checkExec(spec.FFmpegPath, "ffmpeg"))
	checks = append(checks, checkExec(spec.FFprobePath, "ffprobe"))
	checks = append(checks, checkTempDir())
	apiKeyEnv := strings.TrimSpace(spec.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	if strings.TrimSpace(apiKey) == "" {
		checks = append(checks, domain.DoctorCheck{
			Name:    "openai_api_key",
			OK:      false,
			Message: fmt.Sprintf("%s not found. Export it before transcription, for example: export %s=\"sk-...\"", apiKeyEnv, apiKeyEnv),
			Code:    "missing_api_key",
		})
	} else {
		checks = append(checks, domain.DoctorCheck{Name: "openai_api_key", OK: true, Message: fmt.Sprintf("API key found in %s", apiKeyEnv)})
		if s.Provider == nil {
			checks = append(checks, domain.DoctorCheck{Name: "provider_connectivity", OK: false, Message: "provider is not configured", Code: "missing_api_key"})
		} else if err := s.Provider.CheckConnectivity(ctx); err != nil {
			checks = append(checks, domain.DoctorCheck{Name: "provider_connectivity", OK: false, Message: err.Error(), Code: domain.ErrorCode(err)})
		} else {
			checks = append(checks, domain.DoctorCheck{Name: "provider_connectivity", OK: true, Message: "provider reachable"})
		}
	}
	return domain.DoctorResult{
		ProtocolVersion: domain.ProtocolVersion,
		GeneratedAt:     time.Now().UTC(),
		Checks:          checks,
		Warnings: []domain.Warning{{
			Code:    "audio_upload_notice",
			Message: "Audio data is sent to the configured provider during transcription and doctor connectivity checks.",
		}},
	}, nil
}

func checkExec(bin, name string) domain.DoctorCheck {
	if bin == "" {
		bin = name
	}
	if strings.Contains(bin, string(filepath.Separator)) {
		if _, err := os.Stat(bin); err == nil {
			return domain.DoctorCheck{Name: name, OK: true, Message: fmt.Sprintf("%s found", bin)}
		}
		return domain.DoctorCheck{Name: name, OK: false, Message: fmt.Sprintf("%s not found", bin), Code: "missing_dependency"}
	}
	if _, err := exec.LookPath(bin); err == nil {
		return domain.DoctorCheck{Name: name, OK: true, Message: fmt.Sprintf("%s resolvable via PATH", bin)}
	}
	return domain.DoctorCheck{Name: name, OK: false, Message: fmt.Sprintf("%s not found", bin), Code: "missing_dependency"}
}

func checkTempDir() domain.DoctorCheck {
	f, err := os.CreateTemp("", "transcriber-doctor-*")
	if err != nil {
		return domain.DoctorCheck{Name: "temp_dir", OK: false, Message: "temporary directory is not writable", Code: "temp_unwritable"}
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	return domain.DoctorCheck{Name: "temp_dir", OK: true, Message: "temporary directory is writable"}
}

func (s *Services) DryRun(ctx context.Context, spec domain.JobSpec) (map[string]any, error) {
	probe, execPlan, err := s.Probe(ctx, spec)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"protocol_version": domain.ProtocolVersion,
		"job_id":           spec.JobID,
		"input":            spec.InputPath,
		"probe":            probe,
		"plan":             execPlan,
	}, nil
}

func (s *Services) Transcribe(ctx context.Context, spec domain.JobSpec) (artifacts []domain.Artifact, stdoutData []byte, err error) {
	start := time.Now()
	s.Logger.Info("starting transcription job", map[string]any{"job_id": spec.JobID, "input_path": spec.InputPath, "model": spec.Model})

	if err = s.Events.Emit("job.started", map[string]any{"input": spec.InputPath}); err != nil {
		return nil, nil, err
	}
	_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageProbing, "message": "probing input"})
	probeStarted := time.Now()
	probe, execPlan, err := s.Probe(ctx, spec)
	if err != nil {
		_ = s.emitFailure(ctx, err)
		return nil, nil, err
	}
	timings := domain.Timings{Probe: time.Since(probeStarted).Milliseconds()}
	_ = s.Events.Emit("job.planned", map[string]any{"input": spec.InputPath, "plan": execPlan})
	if spec.DryRun {
		data, _ := json.MarshalIndent(map[string]any{"probe": probe, "plan": execPlan}, "", "  ")
		return nil, data, nil
	}

	workdir := spec.Workdir
	if workdir == "" {
		dir, mkErr := os.MkdirTemp("", "transcriber-*")
		if mkErr != nil {
			err = mkErr
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
		workdir = dir
	}
	if !spec.KeepWorkdir {
		defer func() { _ = os.RemoveAll(workdir) }()
	}

	var transcripts []domain.Transcript
	var mergePlans []domain.ChunkPlan
	var rawJSONs []string
	var runtimeWarnings []domain.Warning
	knownSpeakerNames := speakerReferenceNames(spec.SpeakerRefs)
	if execPlan.ChunkingMode == domain.ChunkingClient && len(execPlan.Chunks) > 0 {
		_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageNormalizing, "message": "creating client chunks"})
		normalizeStarted := time.Now()
		chunkPlan := execPlan.Chunks
		if s.Optimizer != nil {
			// Boundary optimization is optional and must never hide the original
			// time-based chunk plan if the optimizer cannot help.
			optimized, chunkWarnings, optimizeErr := s.Optimizer.Optimize(ctx, spec, chunkPlan)
			if optimizeErr != nil {
				chunkWarnings = append(chunkWarnings, domain.Warning{
					Code:    "chunk_optimizer_fallback",
					Message: "chunk boundary optimizer failed; falling back to time-based chunking",
				})
				s.Logger.Warn("chunk optimizer fallback", map[string]any{"error": optimizeErr.Error()})
			} else {
				chunkPlan = optimized
			}
			for _, warning := range chunkWarnings {
				runtimeWarnings = domain.AppendWarning(runtimeWarnings, warning)
				_ = s.Events.Emit("warning", map[string]any{"code": warning.Code, "message": warning.Message})
				s.Logger.Warn("chunk preparation warning", map[string]any{"code": warning.Code, "message": warning.Message})
			}
		}
		chunkPaths, chunkErr := s.Media.Chunk(ctx, spec, chunkPlan, workdir)
		timings.Normalize = time.Since(normalizeStarted).Milliseconds()
		if chunkErr != nil {
			err = chunkErr
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}

		_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageTranscribing, "message": "transcribing chunks"})
		transcribeStarted := time.Now()
		transcripts = make([]domain.Transcript, len(chunkPaths))
		mergePlans = slices.Clone(chunkPlan)
		rawJSONs = make([]string, len(chunkPaths))
		if isWhisperParallel(spec) {
			var (
				wg        sync.WaitGroup
				mu        sync.Mutex
				firstErr  error
				completed int
				okChunks  = make([]bool, len(chunkPaths))
			)
			for i, chunkPath := range chunkPaths {
				i, chunkPath := i, chunkPath
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = s.Events.Emit("chunk.started", map[string]any{"chunk_index": i, "path": chunkPath})
					resp, transcribeErr := s.Provider.Transcribe(ctx, domain.ProviderRequest{
						Spec:           spec,
						FilePath:       chunkPath,
						ResponseFormat: execPlan.ResponseFormat,
					})
					mu.Lock()
					defer mu.Unlock()
					if transcribeErr != nil {
						if firstErr == nil {
							firstErr = transcribeErr
						}
						return
					}
					transcripts[i] = merge.PrepareChunkTranscript(merge.ChunkPreparationOptions{
						Model:                      spec.Model,
						ChunkIndex:                 i,
						TotalChunks:                len(chunkPaths),
						AllowExperimentalStitching: spec.AllowExperimentalDiarizeStitching,
						KnownSpeakerNames:          knownSpeakerNames,
					}, resp.Transcript)
					rawJSONs[i] = resp.RawJSON
					okChunks[i] = true
					completed++
					_ = s.Events.Emit("chunk.completed", map[string]any{"chunk_index": i})
					_ = s.Events.Emit("stage.progress", map[string]any{
						"stage":         domain.StageTranscribing,
						"progress":      float64(completed) / float64(len(chunkPaths)),
						"current_chunk": completed,
						"total_chunks":  len(chunkPaths),
						"message":       fmt.Sprintf("sending chunk %d/%d", completed, len(chunkPaths)),
					})
				}()
			}
			wg.Wait()
			if firstErr != nil {
				filteredTranscripts, filteredPlans := filterSuccessfulChunks(transcripts, mergePlans)
				return s.handlePartial(ctx, spec, filteredTranscripts, filteredPlans, execPlan, timings, firstErr)
			}
		} else {
			for i, chunkPath := range chunkPaths {
				chunkSpec := spec
				if i > 0 && promptCarryover(chunkPlan, i) {
					chunkSpec.Prompt = joinPrompt(spec.Prompt, tail(transcripts[i-1].Text, 300))
				}
				_ = s.Events.Emit("chunk.started", map[string]any{"chunk_index": i, "path": chunkPath})
				resp, transcribeErr := s.Provider.Transcribe(ctx, domain.ProviderRequest{
					Spec:           chunkSpec,
					FilePath:       chunkPath,
					ResponseFormat: execPlan.ResponseFormat,
				})
				if transcribeErr != nil {
					return s.handlePartial(ctx, spec, transcripts[:i], mergePlans[:i], execPlan, timings, transcribeErr)
				}
				_ = s.Events.Emit("chunk.completed", map[string]any{"chunk_index": i})
				transcripts[i] = merge.PrepareChunkTranscript(merge.ChunkPreparationOptions{
					Model:                      spec.Model,
					ChunkIndex:                 i,
					TotalChunks:                len(chunkPaths),
					AllowExperimentalStitching: spec.AllowExperimentalDiarizeStitching,
					KnownSpeakerNames:          knownSpeakerNames,
				}, resp.Transcript)
				rawJSONs[i] = resp.RawJSON
				_ = s.Events.Emit("stage.progress", map[string]any{
					"stage":         domain.StageTranscribing,
					"progress":      float64(i+1) / float64(len(chunkPaths)),
					"current_chunk": i + 1,
					"total_chunks":  len(chunkPaths),
					"message":       fmt.Sprintf("sending chunk %d/%d", i+1, len(chunkPaths)),
				})
			}
		}
		timings.Transcribe = time.Since(transcribeStarted).Milliseconds()
	} else {
		filePath := spec.InputPath
		if execPlan.FFmpegRequired && (spec.StartSec != nil || spec.EndSec != nil || domain.IsVideoInput(spec.InputPath) || !domain.ProviderCompatibleAudioExtension(spec.InputPath)) {
			_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageNormalizing, "message": "normalizing input"})
			normalizeStarted := time.Now()
			filePath, err = s.Media.Normalize(ctx, spec, workdir)
			timings.Normalize = time.Since(normalizeStarted).Milliseconds()
			if err != nil {
				_ = s.emitFailure(ctx, err)
				return nil, nil, err
			}
			info, statErr := os.Stat(filePath)
			if statErr != nil {
				err = domain.NewError("normalized_input_unreadable", "failed to read normalized audio file", domain.ExitDecode, statErr)
				_ = s.emitFailure(ctx, err)
				return nil, nil, err
			}
			if info.Size() > domain.ProviderUploadLimitBytes {
				err = domain.NewError("normalized_input_too_large", "normalized audio exceeds the single-request upload limit", domain.ExitInput, nil)
				_ = s.emitFailure(ctx, err)
				return nil, nil, err
			}
		}
		_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageTranscribing, "message": "transcribing audio"})
		transcribeStarted := time.Now()
		resp, transcribeErr := s.Provider.Transcribe(ctx, domain.ProviderRequest{
			Spec:           spec,
			FilePath:       filePath,
			ResponseFormat: execPlan.ResponseFormat,
		})
		timings.Transcribe = time.Since(transcribeStarted).Milliseconds()
		if transcribeErr != nil {
			err = transcribeErr
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
		transcripts = append(transcripts, resp.Transcript)
		mergePlans = append(mergePlans, domain.ChunkPlan{
			Index:      0,
			StartSec:   0,
			EndSec:     execPlan.Input.DurationSec,
			OverlapSec: 0,
		})
		rawJSONs = append(rawJSONs, resp.RawJSON)
	}

	_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageMerging, "message": "merging results"})
	mergeStarted := time.Now()
	mergeResult := merge.Merge(spec.Model, transcripts, mergePlans)
	transcript := mergeResult.Transcript
	if transcript.DurationSec == 0 && execPlan.Input.DurationSec > 0 {
		transcript.DurationSec = execPlan.Input.DurationSec
	}
	transcript.Warnings = domain.AppendWarnings(transcript.Warnings, runtimeWarnings...)
	timings.Merge = time.Since(mergeStarted).Milliseconds()

	var dict dictionary.Dictionary
	if spec.DictionaryEnabled && spec.DictionaryPath != "" {
		var loadErr error
		dict, loadErr = dictionary.Load(spec.DictionaryPath)
		if loadErr != nil {
			err = domain.NewError("dictionary_invalid", "failed to load dictionary", domain.ExitConfig, loadErr)
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
	}

	_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StagePostprocessing, "message": "postprocessing transcript"})
	postStarted := time.Now()
	postprocessPrompt := spec.PostprocessPrompt
	if dict != nil {
		postprocessPrompt = joinPrompt(postprocessPrompt, dictionary.PromptHints(dict, transcript.Language, 12))
	}
	transcript, err = s.Postprocess.Apply(ctx, transcript, spec.Postprocess, spec.PostprocessModel, postprocessPrompt)
	timings.Postprocess = time.Since(postStarted).Milliseconds()
	if err != nil {
		_ = s.emitFailure(ctx, err)
		return nil, nil, err
	}
	if dict != nil {
		transcript = dictionary.Apply(transcript, dict)
	}

	_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageRendering, "message": "rendering output"})
	renderStarted := time.Now()
	rendered, renderErr := render.Transcript(render.RenderInput{
		Transcript:   transcript,
		SourcePath:   spec.InputPath,
		Format:       spec.Format,
		ChunkingMode: execPlan.ChunkingMode,
	}, spec.IncludeSegments)
	timings.Render = time.Since(renderStarted).Milliseconds()
	if renderErr != nil {
		err = domain.NewError("render_failed", "failed to render output", domain.ExitWrite, renderErr)
		_ = s.emitFailure(ctx, err)
		return nil, nil, err
	}

	if spec.Stdout {
		_ = s.Events.Emit("job.completed", map[string]any{"artifacts": []domain.Artifact{}})
		s.Logger.Info("transcription job completed", map[string]any{"job_id": spec.JobID, "duration_ms": time.Since(start).Milliseconds()})
		return nil, rendered, nil
	}

	_ = s.Events.Emit("stage.started", map[string]any{"stage": domain.StageWriting, "message": "writing output"})
	writeStarted := time.Now()
	artifacts = execPlan.Artifacts
	if len(artifacts) == 0 {
		artifacts = plan.BuildArtifacts(spec)
	}
	for i := range artifacts {
		if writeErr := fs.WriteFile(artifacts[i].Path, rendered, spec.Overwrite); writeErr != nil {
			err = domain.NewError("write_failed", "failed to write transcript", domain.ExitWrite, writeErr)
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
		info, _ := os.Stat(artifacts[i].Path)
		if info != nil {
			artifacts[i].Bytes = info.Size()
		}
		_ = s.Events.Emit("artifact.written", map[string]any{"artifact": artifacts[i]})
	}
	timings.Write = time.Since(writeStarted).Milliseconds()
	if spec.WriteManifest && len(artifacts) > 0 {
		manifest := domain.Manifest{
			Version:          domain.SchemaVersion,
			JobID:            spec.JobID,
			Input:            execPlan.Input,
			Plan:             domain.PlanSummary{ChunkingMode: execPlan.ChunkingMode, Model: spec.Model, Language: spec.Language},
			Artifacts:        artifacts,
			TimingsMS:        timings,
			Warnings:         transcript.Warnings,
			MergeDiagnostics: mergeResult.Diagnostics,
		}
		manifestData, _ := json.MarshalIndent(manifest, "", "  ")
		if writeErr := fs.WriteFile(artifacts[0].ManifestPath, manifestData, spec.Overwrite); writeErr != nil {
			err = domain.NewError("write_failed", "failed to write manifest", domain.ExitWrite, writeErr)
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
		_ = s.Events.Emit("artifact.written", map[string]any{"artifact": domain.Artifact{Path: artifacts[0].ManifestPath, Format: domain.FormatJSON, Partial: false}})
	}
	if spec.RawProviderJSONPath != "" && len(rawJSONs) > 0 {
		data := marshalRawProviderJSON(rawJSONs)
		if writeErr := fs.WriteFile(spec.RawProviderJSONPath, data, spec.Overwrite); writeErr != nil {
			err = domain.NewError("write_failed", "failed to write raw provider JSON", domain.ExitWrite, writeErr)
			_ = s.emitFailure(ctx, err)
			return nil, nil, err
		}
	}
	_ = s.Events.Emit("job.completed", map[string]any{"artifacts": artifacts, "duration_ms": time.Since(start).Milliseconds()})
	s.Logger.Info("transcription job completed", map[string]any{"job_id": spec.JobID, "artifact_count": len(artifacts), "duration_ms": time.Since(start).Milliseconds()})
	return artifacts, nil, nil
}

func promptCarryover(chunks []domain.ChunkPlan, i int) bool {
	if i >= len(chunks) {
		return false
	}
	return chunks[i].PromptCarryover
}

func tail(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[len(runes)-max:])
}

func joinPrompt(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n\n")
}

func (s *Services) handlePartial(ctx context.Context, spec domain.JobSpec, transcripts []domain.Transcript, mergePlans []domain.ChunkPlan, execPlan plan.ExecutionPlan, _ domain.Timings, cause error) ([]domain.Artifact, []byte, error) {
	if len(transcripts) == 0 || spec.PartialOutput == domain.PartialDiscard {
		_ = s.emitFailure(ctx, cause)
		return nil, nil, cause
	}
	transcript := merge.Merge(spec.Model, transcripts, mergePlans).Transcript
	transcript.Partial = true
	rendered, err := render.Transcript(render.RenderInput{
		Transcript:   transcript,
		SourcePath:   spec.InputPath,
		Format:       spec.Format,
		ChunkingMode: execPlan.ChunkingMode,
	}, spec.IncludeSegments)
	if err != nil {
		_ = s.emitFailure(ctx, cause)
		return nil, nil, cause
	}
	if spec.Stdout || spec.PartialOutput == domain.PartialStdout {
		_ = s.Events.Emit("job.partial", map[string]any{"code": domain.ErrorCode(cause)})
		if ctxCancelled(ctx, cause) {
			_ = s.Events.Emit("job.cancelled", map[string]any{"code": "cancelled"})
		}
		s.Logger.Warn("partial transcription returned to stdout", map[string]any{"job_id": spec.JobID, "code": domain.ErrorCode(cause)})
		return nil, rendered, domain.NewError(domain.ErrorCode(cause), cause.Error(), domain.ExitPartial, cause)
	}
	artifact := plan.PartialArtifact(spec)
	if err := fs.WriteFile(artifact.Path, rendered, true); err != nil {
		_ = s.emitFailure(ctx, domain.NewError("write_failed", "failed to write partial transcript", domain.ExitWrite, err))
		return nil, nil, domain.NewError("write_failed", "failed to write partial transcript", domain.ExitWrite, err)
	}
	_ = s.Events.Emit("artifact.written", map[string]any{"artifact": artifact})
	_ = s.Events.Emit("job.partial", map[string]any{"code": domain.ErrorCode(cause), "artifacts": []domain.Artifact{artifact}})
	if ctxCancelled(ctx, cause) {
		_ = s.Events.Emit("job.cancelled", map[string]any{"code": "cancelled", "artifacts": []domain.Artifact{artifact}})
	}
	s.Logger.Warn("partial transcription written", map[string]any{"job_id": spec.JobID, "artifact_path": artifact.Path, "code": domain.ErrorCode(cause)})
	return []domain.Artifact{artifact}, nil, domain.NewError(domain.ErrorCode(cause), cause.Error(), domain.ExitPartial, cause)
}

func (s *Services) emitFailure(ctx context.Context, err error) error {
	if ctxCancelled(ctx, err) {
		s.Logger.Warn("job cancelled", map[string]any{"code": "cancelled"})
		return s.Events.Emit("job.cancelled", map[string]any{"code": "cancelled"})
	}
	s.Logger.Error("transcription job failed", map[string]any{"code": domain.ErrorCode(err), "error": err.Error()})
	return s.Events.Emit("job.failed", map[string]any{"code": domain.ErrorCode(err), "message": err.Error()})
}

func ctxCancelled(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
}

func isWhisperParallel(spec domain.JobSpec) bool {
	profile, ok := domain.ModelProfileFor(spec.Model)
	return ok && profile.ParallelChunking
}

func filterSuccessfulChunks(transcripts []domain.Transcript, plans []domain.ChunkPlan) ([]domain.Transcript, []domain.ChunkPlan) {
	filteredTranscripts := make([]domain.Transcript, 0, len(transcripts))
	filteredPlans := make([]domain.ChunkPlan, 0, len(plans))
	for i := range transcripts {
		if strings.TrimSpace(transcripts[i].Text) == "" && len(transcripts[i].Segments) == 0 {
			continue
		}
		filteredTranscripts = append(filteredTranscripts, transcripts[i])
		if i < len(plans) {
			filteredPlans = append(filteredPlans, plans[i])
		}
	}
	return filteredTranscripts, filteredPlans
}

func marshalRawProviderJSON(rawJSONs []string) []byte {
	filtered := make([]json.RawMessage, 0, len(rawJSONs))
	for _, raw := range rawJSONs {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		filtered = append(filtered, json.RawMessage(raw))
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return []byte(filtered[0])
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return []byte("[]")
	}
	return data
}

func speakerReferenceNames(refs []domain.SpeakerReference) []string {
	if len(refs) == 0 {
		return nil
	}
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}
