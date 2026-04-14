package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared/constant"

	"ai-transcriber-cli/internal/domain"
)

type Provider struct {
	client openai.Client
}

func New(apiKey string) *Provider {
	return &Provider{client: openai.NewClient(option.WithAPIKey(apiKey))}
}

func (p *Provider) Transcribe(ctx context.Context, req domain.ProviderRequest) (domain.ProviderResponse, error) {
	if err := validateRequest(ctx, req); err != nil {
		return domain.ProviderResponse{}, err
	}
	file, err := os.Open(req.FilePath)
	if err != nil {
		return domain.ProviderResponse{}, domain.NewError("input_open_failed", "failed to open input for transcription", domain.ExitInput, err)
	}
	defer func() { _ = file.Close() }()

	params := openai.AudioTranscriptionNewParams{
		File:           file,
		Model:          openai.AudioModel(providerModelName(req.Spec.Model)),
		ResponseFormat: openai.AudioResponseFormat(req.ResponseFormat),
	}
	if req.Spec.Language != "" && req.Spec.Language != "auto" {
		params.Language = param.NewOpt(req.Spec.Language)
	}
	if req.Spec.Prompt != "" && !req.Spec.IsDiarize() {
		params.Prompt = param.NewOpt(req.Spec.Prompt)
	}
	if req.Spec.Logprobs {
		params.Include = []openai.TranscriptionInclude{openai.TranscriptionIncludeLogprobs}
	}
	if req.ResponseFormat == "verbose_json" {
		params.TimestampGranularities = []string{"segment"}
		if req.Spec.Model == "whisper-1" || req.Spec.Model == "whisper-1-ts" {
			params.TimestampGranularities = []string{"segment", "word"}
		}
	}
	setChunking(&params, req.Spec, req.FilePath)
	if req.Spec.IsDiarize() {
		if len(req.Spec.SpeakerRefs) > 0 {
			names, refs, err := encodeSpeakerRefs(req.Spec.SpeakerRefs)
			if err != nil {
				return domain.ProviderResponse{}, err
			}
			params.KnownSpeakerNames = names
			params.KnownSpeakerReferences = refs
		}
	}

	var lastErr error
	attempts := req.Spec.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if _, err := file.Seek(0, 0); err != nil {
				return domain.ProviderResponse{}, domain.NewError("input_seek_failed", "failed to reset input for retry", domain.ExitInput, err)
			}
		}
		resp, err := p.client.Audio.Transcriptions.New(ctx, params)
		if err == nil {
			return normalizeResponse(resp, req.Spec.Model), nil
		}
		lastErr = classify(err)
		var appErr *domain.AppError
		if !domain.As(lastErr, &appErr) || appErr.ExitCode == domain.ExitAuth || appErr.ExitCode == domain.ExitInput {
			break
		}
		if attempt < attempts-1 {
			select {
			case <-ctx.Done():
				return domain.ProviderResponse{}, domain.NewError("timeout", "transcription request timed out", domain.ExitNetwork, ctx.Err())
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return domain.ProviderResponse{}, lastErr
}

func setChunking(params *openai.AudioTranscriptionNewParams, spec domain.JobSpec, filePath string) {
	profile, ok := domain.ModelProfileFor(spec.Model)
	if !ok || !profile.SupportsServerChunking {
		return
	}
	switch spec.ChunkingMode {
	case domain.ChunkingServerAuto:
		params.ChunkingStrategy = openai.AudioTranscriptionNewParamsChunkingStrategyUnion{OfAuto: constant.ValueOf[constant.Auto]()}
	case domain.ChunkingServerVAD:
		params.ChunkingStrategy = openai.AudioTranscriptionNewParamsChunkingStrategyUnion{
			OfAudioTranscriptionNewsChunkingStrategyVadConfig: &openai.AudioTranscriptionNewParamsChunkingStrategyVadConfig{
				Type:              "server_vad",
				Threshold:         param.NewOpt(spec.ServerVADThreshold),
				PrefixPaddingMs:   param.NewOpt(int64(spec.ServerVADPrefixMS)),
				SilenceDurationMs: param.NewOpt(int64(spec.ServerVADSilenceMS)),
			},
		}
	case domain.ChunkingAuto:
		if shouldUseServerAuto(spec, filePath) {
			params.ChunkingStrategy = openai.AudioTranscriptionNewParamsChunkingStrategyUnion{OfAuto: constant.ValueOf[constant.Auto]()}
		}
	}
}

func providerModelName(model string) string {
	if model == "whisper-1-ts" {
		return "whisper-1"
	}
	return model
}

func validateRequest(ctx context.Context, req domain.ProviderRequest) error {
	if err := domain.ValidateModel(req.Spec.Model); err != nil {
		return err
	}
	if err := domain.ValidatePrompt(req.Spec.Model, req.Spec.Prompt); err != nil {
		return err
	}
	if err := domain.ValidateLogprobs(req.Spec.Model, req.Spec.Logprobs); err != nil {
		return err
	}
	if !domain.SupportsResponseFormat(req.Spec.Model, req.ResponseFormat) {
		return domain.NewError("response_format_not_supported", fmt.Sprintf("response format %q is not supported for model %s", req.ResponseFormat, req.Spec.Model), domain.ExitArgs, nil)
	}
	if req.Spec.Logprobs && req.ResponseFormat != "json" {
		return domain.NewError("logprobs_requires_json", "logprobs requires response_format=json", domain.ExitArgs, nil)
	}
	if req.Spec.IsDiarize() && len(req.Spec.SpeakerRefs) > 0 {
		if err := validateSpeakerRefs(ctx, req.Spec); err != nil {
			return err
		}
	}
	return nil
}

func shouldUseServerAuto(spec domain.JobSpec, filePath string) bool {
	if spec.IsDiarize() {
		if info, err := os.Stat(filePath); err == nil && info.Size() > 25*1024*1024 {
			return false
		}
	}
	return true
}

type speakerRefProbe struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func validateSpeakerRefs(ctx context.Context, spec domain.JobSpec) error {
	ffprobe := strings.TrimSpace(spec.FFprobePath)
	if ffprobe == "" {
		ffprobe = "ffprobe"
	}
	for _, ref := range spec.SpeakerRefs {
		durationSec, err := probeAudioDuration(ctx, ffprobe, ref.Path)
		if err != nil {
			return domain.NewError("speaker_ref_probe_failed", "failed to inspect speaker reference audio", domain.ExitInput, err)
		}
		if durationSec < 2 || durationSec > 10 {
			return domain.NewError("speaker_ref_duration_invalid", fmt.Sprintf("speaker reference %q must be between 2 and 10 seconds", ref.Name), domain.ExitArgs, nil)
		}
	}
	return nil
}

func probeAudioDuration(ctx context.Context, ffprobePath, path string) (float64, error) {
	out, err := exec.CommandContext(ctx, ffprobePath, "-v", "quiet", "-print_format", "json", "-show_format", path).Output()
	if err != nil {
		return 0, err
	}
	var probe speakerRefProbe
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, err
	}
	var duration float64
	if _, err := fmt.Sscanf(probe.Format.Duration, "%f", &duration); err != nil {
		return 0, err
	}
	return math.Round(duration*1000) / 1000, nil
}

func encodeSpeakerRefs(refs []domain.SpeakerReference) ([]string, []string, error) {
	if len(refs) > 4 {
		return nil, nil, domain.NewError("speaker_ref_limit", "up to 4 speaker references are supported", domain.ExitArgs, nil)
	}
	names := make([]string, 0, len(refs))
	dataURLs := make([]string, 0, len(refs))
	for _, ref := range refs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			return nil, nil, err
		}
		mimeType := mimeTypeForAudioPath(ref.Path)
		names = append(names, ref.Name)
		dataURLs = append(dataURLs, fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)))
	}
	return names, dataURLs, nil
}

func normalizeResponse(resp *openai.AudioTranscriptionNewResponseUnion, model string) domain.ProviderResponse {
	out := domain.ProviderResponse{Transcript: domain.Transcript{Version: domain.SchemaVersion, ModelUsed: model, Language: "auto"}}
	if resp == nil {
		return out
	}
	raw := resp.RawJSON()
	out.RawJSON = raw
	if model == "gpt-4o-transcribe-diarize" && strings.Contains(raw, "\"speaker\"") {
		// The generated SDK union does not expose a dedicated diarized response type,
		// so we normalize the raw JSON into our canonical transcript shape here.
		if transcript, ok := parseDiarizedResponse(raw, model); ok {
			out.Transcript = transcript
			return out
		}
	}
	if strings.Contains(raw, "\"segments\"") || strings.Contains(raw, "\"duration\"") {
		v := resp.AsTranscriptionVerbose()
		out.Transcript.Text = v.Text
		out.Transcript.Language = v.Language
		out.Transcript.DurationSec = v.Duration
		for _, seg := range v.Segments {
			out.Transcript.Segments = append(out.Transcript.Segments, domain.Segment{
				ID:       fmt.Sprintf("seg-%d", seg.ID),
				StartSec: seg.Start,
				EndSec:   seg.End,
				Text:     seg.Text,
			})
		}
		for _, word := range v.Words {
			out.Transcript.Words = append(out.Transcript.Words, domain.WordTiming{
				StartSec: word.Start,
				EndSec:   word.End,
				Word:     word.Word,
			})
		}
		out.Transcript.Usage = usageFromVerbose(v.Usage)
		return out
	}
	t := resp.AsTranscription()
	out.Transcript.Text = t.Text
	out.Transcript.Usage = usageFromUnion(t.Usage)
	return out
}

func usageFromUnion(u openai.TranscriptionUsageUnion) domain.Usage {
	result := domain.Usage{Type: u.Type}
	switch v := u.AsAny().(type) {
	case openai.TranscriptionUsageTokens:
		result.InputTokens = v.InputTokens
		result.OutputTokens = v.OutputTokens
		result.TotalTokens = v.TotalTokens
		result.AudioTokens = v.InputTokenDetails.AudioTokens
		result.TextTokens = v.InputTokenDetails.TextTokens
	case openai.TranscriptionUsageDuration:
		result.Type = string(v.Type)
		result.Seconds = v.Seconds
	}
	return result
}

func usageFromVerbose(u openai.TranscriptionVerboseUsage) domain.Usage {
	return domain.Usage{
		Type:    string(u.Type),
		Seconds: u.Seconds,
	}
}

type diarizedResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Segments []struct {
		ID      any     `json:"id"`
		Start   float64 `json:"start"`
		End     float64 `json:"end"`
		Text    string  `json:"text"`
		Speaker string  `json:"speaker"`
	} `json:"segments"`
	Usage json.RawMessage `json:"usage"`
}

func parseDiarizedResponse(raw, model string) (domain.Transcript, bool) {
	var payload diarizedResponse
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return domain.Transcript{}, false
	}
	transcript := domain.Transcript{
		Version:     domain.SchemaVersion,
		ModelUsed:   model,
		Text:        payload.Text,
		Language:    payload.Language,
		DurationSec: payload.Duration,
	}
	seenSpeakers := make(map[string]struct{})
	for i, seg := range payload.Segments {
		var speakerPtr *string
		if strings.TrimSpace(seg.Speaker) != "" {
			speaker := seg.Speaker
			speakerPtr = &speaker
			if _, exists := seenSpeakers[speaker]; !exists {
				seenSpeakers[speaker] = struct{}{}
				transcript.Speakers = append(transcript.Speakers, speaker)
			}
		}
		transcript.Segments = append(transcript.Segments, domain.Segment{
			ID:       segmentID(seg.ID, i),
			StartSec: seg.Start,
			EndSec:   seg.End,
			Text:     seg.Text,
			Speaker:  speakerPtr,
		})
	}
	return transcript, true
}

func segmentID(raw any, fallback int) string {
	switch v := raw.(type) {
	case string:
		if v != "" {
			return v
		}
	case float64:
		return fmt.Sprintf("seg-%d", int(v))
	}
	return fmt.Sprintf("seg-%d", fallback)
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return domain.NewError("auth_error", "authentication failed", domain.ExitAuth, err)
		case 429:
			return domain.NewError("rate_limit", "rate limited by provider", domain.ExitRateLimit, err)
		}
		if apiErr.StatusCode >= 500 {
			return domain.NewError("provider_error", "provider request failed", domain.ExitProvider, err)
		}
		if apiErr.StatusCode >= 400 {
			return domain.NewError("provider_error", "provider request failed", domain.ExitProvider, err)
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalid api key"):
		return domain.NewError("auth_error", "authentication failed", domain.ExitAuth, err)
	case strings.Contains(msg, "429"):
		return domain.NewError("rate_limit", "rate limited by provider", domain.ExitRateLimit, err)
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "connection"), strings.Contains(msg, "dial tcp"):
		return domain.NewError("network_error", "network error", domain.ExitNetwork, err)
	default:
		return domain.NewError("provider_error", "provider request failed", domain.ExitProvider, err)
	}
}

func mimeTypeForAudioPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4a":
		return "audio/mp4"
	case ".mp3", ".mpga", ".mpeg":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".webm":
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}

func MustTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (p *Provider) CheckConnectivity(ctx context.Context) error {
	_, err := p.client.Models.List(ctx)
	if err != nil {
		return classify(err)
	}
	return nil
}
