package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/events"
	"ai-transcriber-cli/internal/logging"
	"ai-transcriber-cli/internal/plan"
)

type stubMediaService struct {
	input  domain.InputInfo
	chunks []string
}

func (s stubMediaService) Probe(context.Context, string, string) (domain.InputInfo, error) {
	return s.input, nil
}

func (s stubMediaService) Chunk(context.Context, domain.JobSpec, []domain.ChunkPlan, string) ([]string, error) {
	return slices.Clone(s.chunks), nil
}

func (s stubMediaService) Normalize(context.Context, domain.JobSpec, string) (string, error) {
	return "", nil
}

type stubPlanner struct {
	execPlan plan.ExecutionPlan
}

func (s stubPlanner) Build(domain.JobSpec, domain.InputInfo) (plan.ExecutionPlan, error) {
	return s.execPlan, nil
}

type noopPostprocessor struct{}

func (noopPostprocessor) Apply(_ context.Context, t domain.Transcript, _ bool, _, _ string) (domain.Transcript, error) {
	return t, nil
}

type passthroughOptimizer struct {
	warnings []domain.Warning
}

func (o passthroughOptimizer) Optimize(_ context.Context, _ domain.JobSpec, chunks []domain.ChunkPlan) ([]domain.ChunkPlan, []domain.Warning, error) {
	return slices.Clone(chunks), slices.Clone(o.warnings), nil
}

type failingOptimizer struct{}

func (failingOptimizer) Optimize(_ context.Context, _ domain.JobSpec, chunks []domain.ChunkPlan) ([]domain.ChunkPlan, []domain.Warning, error) {
	return slices.Clone(chunks), nil, context.DeadlineExceeded
}

type recordingEvents struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingEvents) Emit(eventType string, _ map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventType)
	return nil
}

func (r *recordingEvents) contains(eventType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Contains(r.events, eventType)
}

type parallelWhisperProvider struct {
	started   chan int
	release   chan struct{}
	mu        sync.Mutex
	requests  []domain.ProviderRequest
	responses []domain.ProviderResponse
}

func (p *parallelWhisperProvider) Transcribe(_ context.Context, req domain.ProviderRequest) (domain.ProviderResponse, error) {
	p.mu.Lock()
	idx := len(p.requests)
	p.requests = append(p.requests, req)
	resp := p.responses[idx]
	p.mu.Unlock()

	p.started <- idx
	<-p.release
	return resp, nil
}

func (p *parallelWhisperProvider) CheckConnectivity(context.Context) error { return nil }

func (p *parallelWhisperProvider) prompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.requests))
	for _, req := range p.requests {
		out = append(out, req.Spec.Prompt)
	}
	return out
}

type sequenceProvider struct {
	mu        sync.Mutex
	responses []domain.ProviderResponse
	requests  []domain.ProviderRequest
}

func (p *sequenceProvider) Transcribe(_ context.Context, req domain.ProviderRequest) (domain.ProviderResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	return p.responses[len(p.requests)-1], nil
}

func (p *sequenceProvider) CheckConnectivity(context.Context) error { return nil }

type cancelOnSecondProvider struct {
	mu       sync.Mutex
	calls    int
	response domain.ProviderResponse
}

func (p *cancelOnSecondProvider) Transcribe(ctx context.Context, req domain.ProviderRequest) (domain.ProviderResponse, error) {
	p.mu.Lock()
	call := p.calls
	p.calls++
	p.mu.Unlock()
	if call == 0 {
		return p.response, nil
	}
	<-ctx.Done()
	return domain.ProviderResponse{}, ctx.Err()
}

func (p *cancelOnSecondProvider) CheckConnectivity(context.Context) error { return nil }

func testLogger() *logging.Logger {
	return logging.New(io.Discard, logging.LevelDebug, domain.LogFormatText)
}

func TestTranscribeWhisperChunksInParallel(t *testing.T) {
	t.Parallel()

	spec := domain.JobSpec{
		JobID:         "job-test",
		InputPath:     "/tmp/input.mp3",
		Format:        domain.FormatTXT,
		Model:         "whisper-1",
		ChunkingMode:  domain.ChunkingClient,
		Stdout:        true,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "verbose_json",
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 25, OverlapSec: 5},
			{Index: 1, StartSec: 20, EndSec: 45, OverlapSec: 5},
		},
	}
	provider := &parallelWhisperProvider{
		started: make(chan int, 2),
		release: make(chan struct{}),
		responses: []domain.ProviderResponse{
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "whisper-1", Language: "ja", Text: "こんにちは"}},
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "whisper-1", Language: "ja", Text: "世界"}},
		},
	}
	recorder := &recordingEvents{}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 45, HasAudio: true},
			chunks: []string{"/tmp/chunk-0.wav", "/tmp/chunk-1.wav"},
		},
		stubPlanner{execPlan: execPlan},
		provider,
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		recorder,
	)

	resultCh := make(chan error, 1)
	var stdout []byte
	go func() {
		_, out, err := svc.Transcribe(context.Background(), spec)
		stdout = out
		resultCh <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-provider.started:
		case <-time.After(2 * time.Second):
			t.Fatal("expected both whisper chunk requests to start before release")
		}
	}
	close(provider.release)

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("Transcribe() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Transcribe() did not finish")
	}

	if string(stdout) == "" {
		t.Fatal("expected stdout transcript output")
	}
	if !recorder.contains("job.completed") {
		t.Fatal("expected job.completed event")
	}
	for _, prompt := range provider.prompts() {
		if prompt != "" {
			t.Fatalf("expected whisper chunk requests to avoid prompt carryover, got %q", prompt)
		}
	}
}

func TestTranscribeFallsBackWhenOptimizerFails(t *testing.T) {
	t.Parallel()

	spec := domain.JobSpec{
		JobID:         "job-optimizer-fallback",
		InputPath:     "/tmp/input.mp3",
		Format:        domain.FormatTXT,
		Model:         "whisper-1",
		ChunkingMode:  domain.ChunkingClient,
		Stdout:        true,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "verbose_json",
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 25, OverlapSec: 5},
			{Index: 1, StartSec: 20, EndSec: 45, OverlapSec: 5},
		},
	}
	recorder := &recordingEvents{}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 45, HasAudio: true},
			chunks: []string{"/tmp/chunk-0.wav", "/tmp/chunk-1.wav"},
		},
		stubPlanner{execPlan: execPlan},
		&sequenceProvider{responses: []domain.ProviderResponse{
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "whisper-1", Language: "en", Text: "hello"}},
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "whisper-1", Language: "en", Text: "world"}},
		}},
		failingOptimizer{},
		noopPostprocessor{},
		testLogger(),
		recorder,
	)

	_, out, err := svc.Transcribe(context.Background(), spec)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("expected transcription output, got %q", string(out))
	}
	if !recorder.contains("warning") {
		t.Fatal("expected warning event when optimizer falls back")
	}
}

func TestTranscribeWritesAllRawProviderJSON(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "out.json")
	rawPath := filepath.Join(tempDir, "raw-provider.json")
	spec := domain.JobSpec{
		JobID:               "job-test",
		InputPath:           filepath.Join(tempDir, "input.mp3"),
		OutputPath:          outputPath,
		Format:              domain.FormatJSON,
		Model:               "gpt-4o-transcribe",
		ChunkingMode:        domain.ChunkingClient,
		Overwrite:           true,
		WriteManifest:       false,
		RawProviderJSONPath: rawPath,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Artifacts: []domain.Artifact{
			{Path: outputPath, Format: domain.FormatJSON},
		},
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 300, OverlapSec: 30, PromptCarryover: true},
			{Index: 1, StartSec: 270, EndSec: 600, OverlapSec: 30, PromptCarryover: true},
		},
	}
	provider := &sequenceProvider{
		responses: []domain.ProviderResponse{
			{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "first"},
				RawJSON:    `{"chunk":1,"text":"first"}`,
			},
			{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "second"},
				RawJSON:    `{"chunk":2,"text":"second"}`,
			},
		},
	}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 600, HasAudio: true},
			chunks: []string{filepath.Join(tempDir, "chunk-0.wav"), filepath.Join(tempDir, "chunk-1.wav")},
		},
		stubPlanner{execPlan: execPlan},
		provider,
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	artifacts, stdout, err := svc.Transcribe(context.Background(), spec)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if len(stdout) != 0 {
		t.Fatalf("expected no stdout output, got %q", string(stdout))
	}
	if len(artifacts) != 1 || artifacts[0].Path != outputPath {
		t.Fatalf("unexpected artifacts: %#v", artifacts)
	}

	data, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", rawPath, err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("raw provider JSON should be an array, got %q: %v", string(data), err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 raw responses, got %d", len(payload))
	}
	if int(payload[0]["chunk"].(float64)) != 1 || int(payload[1]["chunk"].(float64)) != 2 {
		t.Fatalf("unexpected raw provider payload: %#v", payload)
	}
}

func TestTranscribeGpt4oCarriesPromptSequentially(t *testing.T) {
	t.Parallel()

	spec := domain.JobSpec{
		JobID:         "job-sequential",
		InputPath:     "/tmp/input.mp3",
		Format:        domain.FormatTXT,
		Model:         "gpt-4o-transcribe",
		Prompt:        "domain terms",
		ChunkingMode:  domain.ChunkingClient,
		Stdout:        true,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 300, OverlapSec: 30, PromptCarryover: true},
			{Index: 1, StartSec: 270, EndSec: 600, OverlapSec: 30, PromptCarryover: true},
		},
	}
	provider := &sequenceProvider{
		responses: []domain.ProviderResponse{
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "最初のチャンク本文"}},
			{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "次のチャンク本文"}},
		},
	}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 600, HasAudio: true},
			chunks: []string{"/tmp/chunk-0.wav", "/tmp/chunk-1.wav"},
		},
		stubPlanner{execPlan: execPlan},
		provider,
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	_, stdout, err := svc.Transcribe(context.Background(), spec)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if len(stdout) == 0 {
		t.Fatal("expected stdout transcript output")
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(provider.requests))
	}
	if provider.requests[0].Spec.Prompt != "domain terms" {
		t.Fatalf("unexpected first prompt: %q", provider.requests[0].Spec.Prompt)
	}
	secondPrompt := provider.requests[1].Spec.Prompt
	if secondPrompt == "" {
		t.Fatal("expected second prompt to include carryover text")
	}
	if secondPrompt == "domain terms" {
		t.Fatalf("expected second prompt to include carryover text, got %q", secondPrompt)
	}
	if !containsAll(secondPrompt, "domain terms", "最初のチャンク本文") {
		t.Fatalf("expected second prompt to contain original prompt and prior transcript, got %q", secondPrompt)
	}
}

func TestTranscribeCancellationWritesPartialAndCancelledEvent(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "out.txt")
	spec := domain.JobSpec{
		JobID:         "job-cancel",
		InputPath:     filepath.Join(tempDir, "input.mp3"),
		OutputPath:    outputPath,
		Format:        domain.FormatTXT,
		Model:         "gpt-4o-transcribe",
		ChunkingMode:  domain.ChunkingClient,
		Overwrite:     true,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Artifacts:      []domain.Artifact{{Path: outputPath, Format: domain.FormatTXT}},
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 300, OverlapSec: 30, PromptCarryover: true},
			{Index: 1, StartSec: 270, EndSec: 600, OverlapSec: 30, PromptCarryover: true},
		},
	}
	recorder := &recordingEvents{}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 600, HasAudio: true},
			chunks: []string{filepath.Join(tempDir, "chunk-0.wav"), filepath.Join(tempDir, "chunk-1.wav")},
		},
		stubPlanner{execPlan: execPlan},
		&cancelOnSecondProvider{
			response: domain.ProviderResponse{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "first chunk"},
			},
		},
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		recorder,
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	artifacts, stdout, err := svc.Transcribe(ctx, spec)
	if err == nil {
		t.Fatal("expected partial/cancel error")
	}
	if domain.ExitCode(err) != domain.ExitPartial {
		t.Fatalf("expected ExitPartial, got %d (%v)", domain.ExitCode(err), err)
	}
	if len(stdout) != 0 {
		t.Fatalf("expected no stdout output, got %q", string(stdout))
	}
	if len(artifacts) != 1 || !artifacts[0].Partial {
		t.Fatalf("expected partial artifact, got %#v", artifacts)
	}
	if _, statErr := os.Stat(artifacts[0].Path); statErr != nil {
		t.Fatalf("expected partial artifact file, stat error = %v", statErr)
	}
	if !recorder.contains("job.partial") {
		t.Fatal("expected job.partial event")
	}
	if !recorder.contains("job.cancelled") {
		t.Fatal("expected job.cancelled event")
	}
}

func TestTranscribeCancellationOverwritesExistingPartialArtifact(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "out.txt")
	partialPath := filepath.Join(tempDir, "out.partial.txt")
	if err := os.WriteFile(partialPath, []byte("old partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := domain.JobSpec{
		JobID:         "job-cancel-overwrite",
		InputPath:     filepath.Join(tempDir, "input.mp3"),
		OutputPath:    outputPath,
		Format:        domain.FormatTXT,
		Model:         "gpt-4o-transcribe",
		ChunkingMode:  domain.ChunkingClient,
		Overwrite:     false,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Artifacts:      []domain.Artifact{{Path: outputPath, Format: domain.FormatTXT}},
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 300, OverlapSec: 30, PromptCarryover: true},
			{Index: 1, StartSec: 270, EndSec: 600, OverlapSec: 30, PromptCarryover: true},
		},
	}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 600, HasAudio: true},
			chunks: []string{filepath.Join(tempDir, "chunk-0.wav"), filepath.Join(tempDir, "chunk-1.wav")},
		},
		stubPlanner{execPlan: execPlan},
		&cancelOnSecondProvider{
			response: domain.ProviderResponse{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "fresh partial"},
			},
		},
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	artifacts, _, err := svc.Transcribe(ctx, spec)
	if err == nil || domain.ExitCode(err) != domain.ExitPartial {
		t.Fatalf("expected ExitPartial, got %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != partialPath {
		t.Fatalf("unexpected partial artifacts: %#v", artifacts)
	}
	data, readErr := os.ReadFile(partialPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) == "old partial" {
		t.Fatalf("expected partial artifact to be overwritten, got %q", string(data))
	}
}

func TestTranscribeRespectsOverwriteForManifestAndRawProviderJSON(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "out.txt")
	manifestPath := filepath.Join(tempDir, "out.manifest.json")
	rawPath := filepath.Join(tempDir, "raw-provider.json")
	if err := os.WriteFile(manifestPath, []byte("existing manifest"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, []byte("existing raw"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := domain.JobSpec{
		JobID:               "job-overwrite",
		InputPath:           filepath.Join(tempDir, "input.mp3"),
		OutputPath:          outputPath,
		Format:              domain.FormatTXT,
		Model:               "gpt-4o-transcribe",
		Overwrite:           false,
		WriteManifest:       true,
		RawProviderJSONPath: rawPath,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingOff,
		ResponseFormat: "json",
		Artifacts: []domain.Artifact{{
			Path:         outputPath,
			Format:       domain.FormatTXT,
			ManifestPath: manifestPath,
		}},
	}
	svc := New(
		stubMediaService{
			input: domain.InputInfo{Path: spec.InputPath, SizeBytes: 1024, DurationSec: 30, HasAudio: true},
		},
		stubPlanner{execPlan: execPlan},
		&sequenceProvider{
			responses: []domain.ProviderResponse{{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "done"},
				RawJSON:    `{"ok":true}`,
			}},
		},
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	_, _, err := svc.Transcribe(context.Background(), spec)
	if err == nil {
		t.Fatal("expected write error due to overwrite=false")
	}
	if domain.ExitCode(err) != domain.ExitWrite {
		t.Fatalf("expected ExitWrite, got %d (%v)", domain.ExitCode(err), err)
	}
	manifestData, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(manifestData) != "existing manifest" {
		t.Fatalf("manifest was overwritten unexpectedly: %q", string(manifestData))
	}
	rawData, readErr := os.ReadFile(rawPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(rawData) != "existing raw" {
		t.Fatalf("raw provider json was overwritten unexpectedly: %q", string(rawData))
	}
}

func TestTranscribeWritesMergeDiagnosticsToManifest(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "out.json")
	manifestPath := filepath.Join(tempDir, "out.manifest.json")
	spec := domain.JobSpec{
		JobID:         "job-merge-diagnostics",
		InputPath:     filepath.Join(tempDir, "input.mp3"),
		OutputPath:    outputPath,
		Format:        domain.FormatJSON,
		Model:         "gpt-4o-mini-transcribe",
		ChunkingMode:  domain.ChunkingClient,
		Overwrite:     true,
		WriteManifest: true,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Artifacts: []domain.Artifact{{
			Path:         outputPath,
			Format:       domain.FormatJSON,
			ManifestPath: manifestPath,
		}},
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 60, OverlapSec: 30, PromptCarryover: true},
			{Index: 1, StartSec: 30, EndSec: 90, OverlapSec: 30, PromptCarryover: true},
		},
	}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 570, HasAudio: true},
			chunks: []string{filepath.Join(tempDir, "chunk-0.wav"), filepath.Join(tempDir, "chunk-1.wav")},
		},
		stubPlanner{execPlan: execPlan},
		&sequenceProvider{
			responses: []domain.ProviderResponse{
				{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-mini-transcribe", Language: "ja", Text: "本日は健康づくりの話をします。大阪に行くので、今後大阪府において健康づくりを広げます。"}},
				{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-mini-transcribe", Language: "ja", Text: "その中で大阪に行くので、今後大阪府において健康づくりを広げます。次にフレイル予防の話に進みます。"}},
			},
		},
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	if _, _, err := svc.Transcribe(context.Background(), spec); err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", manifestPath, err)
	}
	var manifest domain.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON error = %v", err)
	}
	if len(manifest.MergeDiagnostics) != 1 {
		t.Fatalf("expected one merge diagnostic, got %#v", manifest.MergeDiagnostics)
	}
	if manifest.MergeDiagnostics[0].Strategy != "v2_trigram" {
		t.Fatalf("expected v2_trigram merge strategy, got %#v", manifest.MergeDiagnostics[0])
	}
}

func TestTranscribeEmitsChunkPreparationWarnings(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	recorder := &recordingEvents{}
	spec := domain.JobSpec{
		JobID:         "job-warning",
		InputPath:     filepath.Join(tempDir, "input.mp3"),
		OutputPath:    filepath.Join(tempDir, "out.txt"),
		Format:        domain.FormatTXT,
		Model:         "gpt-4o-transcribe",
		ChunkingMode:  domain.ChunkingClient,
		Overwrite:     true,
		WriteManifest: false,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:   domain.ChunkingClient,
		ResponseFormat: "json",
		Artifacts:      []domain.Artifact{{Path: spec.OutputPath, Format: domain.FormatTXT}},
		Chunks: []domain.ChunkPlan{
			{Index: 0, StartSec: 0, EndSec: 300, OverlapSec: 30, PromptCarryover: true},
		},
	}
	svc := New(
		stubMediaService{
			input:  domain.InputInfo{Path: spec.InputPath, SizeBytes: 30 << 20, DurationSec: 300, HasAudio: true},
			chunks: []string{filepath.Join(tempDir, "chunk-0.wav")},
		},
		stubPlanner{execPlan: execPlan},
		&sequenceProvider{
			responses: []domain.ProviderResponse{{
				Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: "gpt-4o-transcribe", Language: "ja", Text: "done"},
			}},
		},
		passthroughOptimizer{warnings: []domain.Warning{{
			Code:    "local_vad_fallback",
			Message: "local VAD boundary optimization was unavailable; falling back to time-based chunking",
		}}},
		noopPostprocessor{},
		testLogger(),
		recorder,
	)

	if _, _, err := svc.Transcribe(context.Background(), spec); err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if !recorder.contains("warning") {
		t.Fatal("expected warning event from chunk preparation warning")
	}
}

func TestDryRunReturnsProbeAndPlan(t *testing.T) {
	t.Parallel()

	spec := domain.JobSpec{
		JobID:        "job-dry-run",
		InputPath:    "/tmp/input.mp3",
		Format:       domain.FormatMD,
		Model:        "gpt-4o-transcribe",
		DryRun:       true,
		EventsMode:   domain.EventsNone,
		LogFormat:    domain.LogFormatText,
		ChunkingMode: domain.ChunkingAuto,
	}
	execPlan := plan.ExecutionPlan{
		ChunkingMode:              domain.ChunkingServerAuto,
		ResponseFormat:            "json",
		SingleRequestPossible:     true,
		TimestampCapable:          false,
		DiarizeCapable:            false,
		EstimatedIntermediateSize: 0,
	}
	svc := New(
		stubMediaService{
			input: domain.InputInfo{Path: spec.InputPath, SizeBytes: 1024, DurationSec: 30, HasAudio: true, Container: "mp3"},
		},
		stubPlanner{execPlan: execPlan},
		&sequenceProvider{},
		passthroughOptimizer{},
		noopPostprocessor{},
		testLogger(),
		events.NopWriter{},
	)

	data, err := svc.DryRun(context.Background(), spec)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}
	if data["job_id"] != spec.JobID {
		t.Fatalf("job_id = %#v, want %q", data["job_id"], spec.JobID)
	}
	if data["input"] != spec.InputPath {
		t.Fatalf("input = %#v, want %q", data["input"], spec.InputPath)
	}
	if _, ok := data["probe"].(domain.ProbeResult); !ok {
		t.Fatalf("probe payload has unexpected type: %T", data["probe"])
	}
	if _, ok := data["plan"].(plan.ExecutionPlan); !ok {
		t.Fatalf("plan payload has unexpected type: %T", data["plan"])
	}
}

func containsAll(text string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}
