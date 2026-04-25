package specbuilder

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
)

type Input struct {
	InputPath                         string
	OutputPath                        string
	OutputDir                         string
	Format                            string
	Model                             string
	Language                          string
	Prompt                            string
	Logprobs                          *bool
	Diarize                           bool
	ChunkingMode                      string
	VADMode                           string
	DictionaryPath                    string
	DictionaryEnabled                 *bool
	Postprocess                       *bool
	PostprocessModel                  string
	PostprocessPrompt                 string
	Start                             string
	End                               string
	EventsMode                        string
	Quiet                             bool
	Verbose                           bool
	LogFormat                         string
	JobID                             string
	FFmpegPath                        string
	FFprobePath                       string
	KeepWorkdir                       *bool
	Workdir                           string
	Resume                            bool
	Parallel                          bool
	Timeout                           time.Duration
	Retries                           int
	PartialOutput                     string
	Overwrite                         *bool
	WriteManifest                     *bool
	RawProviderJSONPath               string
	Stdout                            bool
	DryRun                            bool
	IncludeSegments                   bool
	AllowExperimentalDiarizeStitching bool
	ChunkTargetSec                    *float64
	ChunkOverlapSec                   *float64
	ServerVADThreshold                float64
	ServerVADPrefixMS                 int
	ServerVADSilenceMS                int
	ObsidianVADMode                   string
	SpeakerRefs                       []string
}

type Defaults struct {
	Config config.AppConfig
	CWD    string
	Env    map[string]string
	JobID  string
}

func Build(input Input, defaults Defaults) (domain.JobSpec, error) {
	inputPath, err := resolvePath(defaults.CWD, input.InputPath)
	if err != nil {
		return domain.JobSpec{}, domain.NewError("input_path_invalid", "failed to resolve input path", domain.ExitInput, err)
	}
	if err := validateLogFlags(input.Quiet, input.Verbose); err != nil {
		return domain.JobSpec{}, err
	}
	speakerRefs, err := parseSpeakerRefs(defaults.CWD, input.SpeakerRefs)
	if err != nil {
		return domain.JobSpec{}, err
	}

	cfg := defaults.Config
	spec := domain.JobSpec{
		JobID:                             valueOr(input.JobID, valueOr(defaults.JobID, "job")),
		APIKeyEnv:                         valueOr(cfg.API.KeyEnv, "OPENAI_API_KEY"),
		InputPath:                         inputPath,
		OutputPath:                        input.OutputPath,
		OutputDir:                         input.OutputDir,
		Format:                            domain.OutputFormat(valueOr(input.Format, string(cfg.Transcription.Format))),
		Model:                             valueOr(input.Model, cfg.Transcription.Model),
		Language:                          valueOr(input.Language, cfg.Transcription.Language),
		Prompt:                            valueOr(input.Prompt, cfg.Transcription.Prompt),
		Logprobs:                          boolValue(input.Logprobs, cfg.Transcription.Logprobs),
		ChunkingMode:                      domain.ChunkingMode(valueOr(input.ChunkingMode, string(cfg.Transcription.ChunkingMode))),
		VADMode:                           domain.VADMode(valueOr(input.VADMode, string(cfg.Transcription.VADMode))),
		DictionaryPath:                    valueOr(input.DictionaryPath, cfg.Dictionary.Path),
		DictionaryEnabled:                 boolValue(input.DictionaryEnabled, cfg.Dictionary.Enabled),
		Postprocess:                       boolValue(input.Postprocess, cfg.Postprocess.Enabled),
		PostprocessModel:                  valueOr(input.PostprocessModel, cfg.Postprocess.Model),
		PostprocessPrompt:                 valueOr(input.PostprocessPrompt, cfg.Postprocess.Prompt),
		EventsMode:                        domain.EventsMode(valueOr(input.EventsMode, string(cfg.Events.Mode))),
		LogFormat:                         domain.LogFormat(valueOr(input.LogFormat, string(cfg.Events.LogFormat))),
		Quiet:                             input.Quiet,
		Verbose:                           input.Verbose,
		FFmpegPath:                        valueOr(input.FFmpegPath, envOr(defaults.Env, "TRANSCRIBER_FFMPEG", cfg.Paths.FFmpeg)),
		FFprobePath:                       valueOr(input.FFprobePath, envOr(defaults.Env, "TRANSCRIBER_FFPROBE", cfg.Paths.FFprobe)),
		KeepWorkdir:                       boolValue(input.KeepWorkdir, cfg.Paths.KeepWorkdir),
		Workdir:                           valueOr(input.Workdir, cfg.Paths.Workdir),
		Resume:                            input.Resume,
		Timeout:                           input.Timeout,
		Retries:                           input.Retries,
		Parallel:                          input.Parallel,
		PartialOutput:                     domain.PartialOutputMode(valueOr(input.PartialOutput, string(cfg.Output.PartialOutput))),
		Overwrite:                         boolValue(input.Overwrite, cfg.Output.Overwrite),
		WriteManifest:                     boolValue(input.WriteManifest, cfg.Output.WriteManifest),
		RawProviderJSONPath:               input.RawProviderJSONPath,
		Stdout:                            input.Stdout,
		DryRun:                            input.DryRun,
		IncludeSegments:                   input.IncludeSegments,
		AllowExperimentalDiarizeStitching: input.AllowExperimentalDiarizeStitching,
		ServerVADThreshold:                firstFloat(input.ServerVADThreshold, cfg.ServerVAD.Threshold),
		ServerVADPrefixMS:                 firstInt(input.ServerVADPrefixMS, cfg.ServerVAD.PrefixPaddingMS),
		ServerVADSilenceMS:                firstInt(input.ServerVADSilenceMS, cfg.ServerVAD.SilenceDurationMS),
		SpeakerRefs:                       speakerRefs,
	}
	if input.ChunkTargetSec != nil {
		value := *input.ChunkTargetSec
		spec.ChunkTargetSecOverride = &value
	}
	if input.ChunkOverlapSec != nil {
		value := *input.ChunkOverlapSec
		spec.ChunkOverlapSecOverride = &value
	}
	applyLegacyVADMode(&spec, input.ObsidianVADMode)
	if input.Diarize {
		spec.Model = "gpt-4o-transcribe-diarize"
	}
	if err := domain.ValidateModel(spec.Model); err != nil {
		return domain.JobSpec{}, err
	}
	if err := domain.ValidateSubtitleFormat(spec.Model, spec.Format); err != nil {
		return domain.JobSpec{}, err
	}
	if err := domain.ValidateLogprobs(spec.Model, spec.Logprobs); err != nil {
		return domain.JobSpec{}, err
	}
	if spec.Stdout && spec.EventsMode == domain.EventsJSONL {
		return domain.JobSpec{}, domain.NewError("stdout_events_conflict", "--stdout cannot be used with --events jsonl", domain.ExitArgs, nil)
	}
	if spec.Resume && spec.Stdout {
		return domain.JobSpec{}, domain.NewError("resume_stdout_conflict", "--resume cannot be used with --stdout", domain.ExitArgs, nil)
	}
	if spec.Resume && spec.DryRun {
		return domain.JobSpec{}, domain.NewError("resume_dry_run_conflict", "--resume cannot be used with --dry-run", domain.ExitArgs, nil)
	}
	if spec.Resume && !spec.WriteManifest {
		return domain.JobSpec{}, domain.NewError("resume_manifest_required", "--resume requires --write-manifest", domain.ExitArgs, nil)
	}
	if spec.PartialOutput == domain.PartialStdout && !spec.Stdout {
		return domain.JobSpec{}, domain.NewError("partial_stdout_requires_stdout", "--partial-output stdout requires --stdout", domain.ExitArgs, nil)
	}
	if spec.Parallel && !supportsManualParallel(spec.Model) {
		return domain.JobSpec{}, domain.NewError("parallel_model_not_supported", "--parallel is supported only with gpt-4o-transcribe and gpt-4o-mini-transcribe", domain.ExitArgs, nil)
	}
	if err := domain.ValidatePrompt(spec.Model, spec.Prompt); err != nil {
		return domain.JobSpec{}, err
	}
	if spec.StartSec, err = parseOptionalTime(input.Start); err != nil {
		return domain.JobSpec{}, domain.NewError("start_invalid", "invalid --start value", domain.ExitArgs, err)
	}
	if spec.EndSec, err = parseOptionalTime(input.End); err != nil {
		return domain.JobSpec{}, domain.NewError("end_invalid", "invalid --end value", domain.ExitArgs, err)
	}
	if spec.StartSec != nil && spec.EndSec != nil && *spec.EndSec <= *spec.StartSec {
		return domain.JobSpec{}, domain.NewError("time_range_invalid", "--end must be greater than --start", domain.ExitArgs, nil)
	}
	return spec, nil
}

func resolvePath(cwd, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if strings.TrimSpace(cwd) == "" {
		return filepath.Abs(path)
	}
	return filepath.Clean(filepath.Join(cwd, path)), nil
}

func validateLogFlags(quiet, verbose bool) error {
	if quiet && verbose {
		return domain.NewError("log_flags_conflict", "--quiet and --verbose cannot be used together", domain.ExitArgs, nil)
	}
	return nil
}

func parseSpeakerRefs(cwd string, values []string) ([]domain.SpeakerReference, error) {
	refs := make([]domain.SpeakerReference, 0, len(values))
	for _, value := range values {
		name, path, found := strings.Cut(value, "=")
		if !found {
			return nil, domain.NewError("speaker_ref_invalid", "speaker references must use name=path form", domain.ExitArgs, nil)
		}
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			return nil, domain.NewError("speaker_ref_invalid", "speaker references must include both name and path", domain.ExitArgs, nil)
		}
		resolved, err := resolvePath(cwd, path)
		if err != nil {
			return nil, domain.NewError("speaker_ref_invalid", "failed to resolve speaker reference path", domain.ExitArgs, err)
		}
		refs = append(refs, domain.SpeakerReference{Name: name, Path: resolved})
	}
	return refs, nil
}

func applyLegacyVADMode(spec *domain.JobSpec, mode string) {
	switch mode {
	case "server":
		spec.ChunkingMode = domain.ChunkingServerAuto
		spec.VADMode = domain.VADDisabled
	case "local":
		spec.ChunkingMode = domain.ChunkingClient
		spec.VADMode = domain.VADLocal
	case "disabled":
		spec.ChunkingMode = domain.ChunkingOff
		spec.VADMode = domain.VADDisabled
	}
}

func parseOptionalTime(value string) (*float64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		var total float64
		for _, part := range parts {
			n, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return nil, err
			}
			total = total*60 + n
		}
		return &total, nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func supportsManualParallel(model string) bool {
	switch model {
	case "gpt-4o-transcribe", "gpt-4o-mini-transcribe":
		return true
	default:
		return false
	}
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func envOr(env map[string]string, name, fallback string) string {
	if value := env[name]; value != "" {
		return value
	}
	return fallback
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func firstFloat(value, fallback float64) float64 {
	if value != 0 {
		return value
	}
	return fallback
}

func firstInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
