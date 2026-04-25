package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/fs"
	"ai-transcriber-cli/internal/plan"
)

type chunkResumeStore struct {
	manifest     domain.Manifest
	manifestPath string
	cacheDir     string
	enabled      bool
}

func newChunkResumeStore(spec domain.JobSpec, execPlan plan.ExecutionPlan, artifacts []domain.Artifact, chunkPlan []domain.ChunkPlan) (*chunkResumeStore, error) {
	if len(artifacts) == 0 || artifacts[0].ManifestPath == "" {
		if spec.Resume {
			return nil, domain.NewError("resume_manifest_unavailable", "--resume requires manifest-backed file output", domain.ExitArgs, nil)
		}
		return nil, nil
	}
	store := &chunkResumeStore{
		manifestPath: artifacts[0].ManifestPath,
		cacheDir:     chunkCacheDirForArtifact(artifacts[0].Path),
		enabled:      true,
	}
	if spec.Resume {
		manifest, err := loadResumeManifest(store.manifestPath)
		if err != nil {
			return nil, err
		}
		if err := validateResumeManifest(manifest, spec, execPlan, chunkPlan); err != nil {
			return nil, err
		}
		store.manifest = manifest
		return store, nil
	}
	store.manifest = buildChunkManifest(spec, execPlan, artifacts, checkpointsForPlan(chunkPlan, store.cacheDir), store.cacheDir, true, domain.Timings{}, nil, nil)
	return store, nil
}

func (s *chunkResumeStore) initialize(spec domain.JobSpec) error {
	if s == nil || !s.enabled || spec.Resume {
		return nil
	}
	if !spec.Overwrite {
		if _, err := os.Stat(s.manifestPath); err == nil {
			return domain.NewError("write_failed", "failed to write manifest", domain.ExitWrite, os.ErrExist)
		} else if !os.IsNotExist(err) {
			return domain.NewError("write_failed", "failed to write manifest", domain.ExitWrite, err)
		}
	}
	return s.writeManifest()
}

func (s *chunkResumeStore) hasCompletedChunk(i int) bool {
	if s == nil || i < 0 || i >= len(s.manifest.Chunks) {
		return false
	}
	return s.manifest.Chunks[i].Status == domain.ChunkCompleted
}

func (s *chunkResumeStore) loadChunk(i int) (domain.Transcript, string, error) {
	if s == nil || i < 0 || i >= len(s.manifest.Chunks) {
		return domain.Transcript{}, "", domain.NewError("resume_chunk_invalid", "resume chunk index is invalid", domain.ExitInput, nil)
	}
	chunk := s.manifest.Chunks[i]
	if chunk.Status != domain.ChunkCompleted {
		return domain.Transcript{}, "", domain.NewError("resume_chunk_incomplete", "resume chunk is not marked complete", domain.ExitInput, nil)
	}
	if strings.TrimSpace(chunk.TranscriptPath) == "" {
		return domain.Transcript{}, "", domain.NewError("resume_chunk_missing_transcript", "resume transcript cache is missing", domain.ExitInput, nil)
	}
	data, err := os.ReadFile(chunk.TranscriptPath)
	if err != nil {
		return domain.Transcript{}, "", domain.NewError("resume_chunk_missing_transcript", "resume transcript cache is missing", domain.ExitInput, err)
	}
	var transcript domain.Transcript
	if err := json.Unmarshal(data, &transcript); err != nil {
		return domain.Transcript{}, "", domain.NewError("resume_chunk_invalid_transcript", "resume transcript cache is unreadable", domain.ExitInput, err)
	}
	var raw string
	if strings.TrimSpace(chunk.RawJSONPath) != "" {
		rawData, err := os.ReadFile(chunk.RawJSONPath)
		if err == nil {
			raw = string(rawData)
		}
	}
	return transcript, raw, nil
}

func (s *chunkResumeStore) saveChunk(i int, transcript domain.Transcript, rawJSON string, timings domain.Timings) error {
	if s == nil || i < 0 || i >= len(s.manifest.Chunks) {
		return nil
	}
	chunk := &s.manifest.Chunks[i]
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return domain.NewError("write_failed", "failed to create chunk cache directory", domain.ExitWrite, err)
	}
	transcriptPath := chunk.TranscriptPath
	if strings.TrimSpace(transcriptPath) == "" {
		transcriptPath = chunkTranscriptPath(s.cacheDir, chunk.Index)
		chunk.TranscriptPath = transcriptPath
	}
	data, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return domain.NewError("write_failed", "failed to serialize chunk transcript cache", domain.ExitWrite, err)
	}
	if err := fs.WriteFileAtomic(transcriptPath, data); err != nil {
		return domain.NewError("write_failed", "failed to write chunk transcript cache", domain.ExitWrite, err)
	}
	if strings.TrimSpace(rawJSON) != "" {
		rawPath := chunk.RawJSONPath
		if strings.TrimSpace(rawPath) == "" {
			rawPath = chunkRawJSONPath(s.cacheDir, chunk.Index)
			chunk.RawJSONPath = rawPath
		}
		if err := fs.WriteFileAtomic(rawPath, []byte(rawJSON)); err != nil {
			return domain.NewError("write_failed", "failed to write chunk raw provider JSON cache", domain.ExitWrite, err)
		}
	}
	chunk.Status = domain.ChunkCompleted
	s.manifest.ResumeCapable = true
	s.manifest.ChunkCacheDir = s.cacheDir
	s.manifest.TimingsMS = timings
	return s.writeManifest()
}

func (s *chunkResumeStore) finalize(spec domain.JobSpec, execPlan plan.ExecutionPlan, artifacts []domain.Artifact, timings domain.Timings, warnings []domain.Warning, diagnostics []domain.MergeBoundaryDiagnostic, keepCache bool) error {
	if s == nil || !s.enabled {
		return nil
	}
	checkpoints := slicesCloneCheckpoints(s.manifest.Chunks)
	if !keepCache {
		for i := range checkpoints {
			checkpoints[i].TranscriptPath = ""
			checkpoints[i].RawJSONPath = ""
		}
	}
	s.manifest = buildChunkManifest(spec, execPlan, artifacts, checkpoints, s.cacheDir, keepCache, timings, warnings, diagnostics)
	if !keepCache {
		s.manifest.ResumeCapable = false
	}
	return s.writeManifest()
}

func (s *chunkResumeStore) cleanupCache() error {
	if s == nil || strings.TrimSpace(s.cacheDir) == "" {
		return nil
	}
	if err := os.RemoveAll(s.cacheDir); err != nil {
		return domain.NewError("write_failed", "failed to remove chunk cache directory", domain.ExitWrite, err)
	}
	return nil
}

func (s *chunkResumeStore) writeManifest() error {
	data, err := json.MarshalIndent(s.manifest, "", "  ")
	if err != nil {
		return domain.NewError("write_failed", "failed to serialize manifest", domain.ExitWrite, err)
	}
	if err := fs.WriteFileAtomic(s.manifestPath, data); err != nil {
		return domain.NewError("write_failed", "failed to write manifest", domain.ExitWrite, err)
	}
	return nil
}

func buildChunkManifest(spec domain.JobSpec, execPlan plan.ExecutionPlan, artifacts []domain.Artifact, checkpoints []domain.ChunkCheckpoint, cacheDir string, resumeCapable bool, timings domain.Timings, warnings []domain.Warning, diagnostics []domain.MergeBoundaryDiagnostic) domain.Manifest {
	manifestCacheDir := cacheDir
	if !resumeCapable {
		manifestCacheDir = ""
	}
	return domain.Manifest{
		Version:          domain.SchemaVersion,
		JobID:            spec.JobID,
		Input:            execPlan.Input,
		Plan:             manifestPlanSummary(spec, execPlan),
		ResumeCapable:    resumeCapable,
		ChunkCacheDir:    manifestCacheDir,
		Chunks:           checkpoints,
		Artifacts:        artifacts,
		TimingsMS:        timings,
		Warnings:         warnings,
		MergeDiagnostics: diagnostics,
	}
}

func manifestPlanSummary(spec domain.JobSpec, execPlan plan.ExecutionPlan) domain.PlanSummary {
	return domain.PlanSummary{
		ChunkingMode:                      execPlan.ChunkingMode,
		ChunkExecutionMode:                chunkExecutionMode(spec, execPlan),
		Model:                             spec.Model,
		Language:                          spec.Language,
		Format:                            spec.Format,
		AllowExperimentalDiarizeStitching: spec.AllowExperimentalDiarizeStitching,
	}
}

func checkpointsForPlan(chunkPlan []domain.ChunkPlan, cacheDir string) []domain.ChunkCheckpoint {
	chunks := make([]domain.ChunkCheckpoint, 0, len(chunkPlan))
	for _, chunk := range chunkPlan {
		chunks = append(chunks, domain.ChunkCheckpoint{
			Index:          chunk.Index,
			StartSec:       chunk.StartSec,
			EndSec:         chunk.EndSec,
			OverlapSec:     chunk.OverlapSec,
			Status:         domain.ChunkPending,
			TranscriptPath: chunkTranscriptPath(cacheDir, chunk.Index),
			RawJSONPath:    chunkRawJSONPath(cacheDir, chunk.Index),
		})
	}
	return chunks
}

func chunkCacheDirForArtifact(outputPath string) string {
	base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	return base + ".chunks"
}

func chunkTranscriptPath(cacheDir string, index int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("chunk-%03d.transcript.json", index))
}

func chunkRawJSONPath(cacheDir string, index int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("chunk-%03d.raw.json", index))
}

func loadResumeManifest(path string) (domain.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Manifest{}, domain.NewError("resume_manifest_missing", "no resumable manifest was found for this output", domain.ExitInput, err)
		}
		return domain.Manifest{}, domain.NewError("resume_manifest_unreadable", "failed to read resume manifest", domain.ExitInput, err)
	}
	var manifest domain.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domain.Manifest{}, domain.NewError("resume_manifest_invalid", "failed to parse resume manifest", domain.ExitInput, err)
	}
	return manifest, nil
}

func validateResumeManifest(manifest domain.Manifest, spec domain.JobSpec, execPlan plan.ExecutionPlan, chunkPlan []domain.ChunkPlan) error {
	if !manifest.ResumeCapable || len(manifest.Chunks) == 0 {
		return domain.NewError("resume_unavailable", "the existing manifest does not contain resumable chunk state", domain.ExitInput, nil)
	}
	if manifest.Input.Path != spec.InputPath {
		return resumeMismatch("input path")
	}
	if manifest.Plan.Model != spec.Model {
		return resumeMismatch("model")
	}
	if manifest.Plan.Language != spec.Language {
		return resumeMismatch("language")
	}
	if manifest.Plan.Format != spec.Format {
		return resumeMismatch("format")
	}
	// Services.Transcribe already rejects non-client resume attempts before
	// constructing the store. Keep the manifest-level check here as defense in
	// depth for callers that reuse this validator directly.
	if manifest.Plan.ChunkingMode != domain.ChunkingClient || execPlan.ChunkingMode != domain.ChunkingClient {
		return domain.NewError("resume_chunking_mode_invalid", "resume is supported only for client-side chunking jobs", domain.ExitInput, nil)
	}
	if manifest.Plan.AllowExperimentalDiarizeStitching != spec.AllowExperimentalDiarizeStitching {
		return resumeMismatch("experimental diarize stitching")
	}
	if manifest.Plan.ChunkExecutionMode != "" && manifest.Plan.ChunkExecutionMode != chunkExecutionMode(spec, execPlan) {
		return resumeMismatch("chunk execution mode")
	}
	if len(manifest.Chunks) != len(chunkPlan) {
		return resumeMismatch("chunk plan")
	}
	for i, chunk := range chunkPlan {
		existing := manifest.Chunks[i]
		if existing.Index != chunk.Index || !sameFloat(existing.StartSec, chunk.StartSec) || !sameFloat(existing.EndSec, chunk.EndSec) || !sameFloat(existing.OverlapSec, chunk.OverlapSec) {
			return resumeMismatch("chunk plan")
		}
	}
	return nil
}

func resumeMismatch(field string) error {
	return domain.NewError("resume_manifest_mismatch", fmt.Sprintf("resume manifest does not match current %s", field), domain.ExitInput, nil)
}

func sameFloat(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func slicesCloneCheckpoints(in []domain.ChunkCheckpoint) []domain.ChunkCheckpoint {
	out := make([]domain.ChunkCheckpoint, len(in))
	copy(out, in)
	return out
}
