package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ai-transcriber-cli/internal/app"
	"ai-transcriber-cli/internal/buildinfo"
	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/events"
	"ai-transcriber-cli/internal/logging"
	"ai-transcriber-cli/internal/media"
	"ai-transcriber-cli/internal/plan"
	"ai-transcriber-cli/internal/postprocess"
	openaip "ai-transcriber-cli/internal/provider/openai"
	"ai-transcriber-cli/internal/vad"
)

type cliState struct {
	cfg     config.AppConfig
	spec    domain.JobSpec
	cfgPath string
}

func main() {
	root := buildRootForTest(os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(domain.ExitCode(err))
	}
}

func buildRootForTest(stdout, stderr io.Writer) *cobra.Command {
	state := &cliState{}
	resolvedConfigPath, configSource := resolveConfigPath(state.cfgPath)
	loadedConfig, _ := config.Load(state.cfgPath)
	root := &cobra.Command{
		Use:   "transcriber",
		Short: "CLI for transcribing audio and video files",
		Long: strings.TrimSpace(fmt.Sprintf(`Transcribe audio and video files via the OpenAI speech-to-text API.

Runtime dependency status on this machine:
  ffmpeg:  %s
  ffprobe: %s

Config status on this machine:
  config path: %s
  config file: %s
  config source: %s
  current defaults: model=%s, format=%s, chunking=%s, vad=%s, events=%s

Recommended flow:
  1. Run "transcriber doctor"
  2. Run "transcriber probe <input>"
  3. Run "transcriber transcribe <input>"

Run "transcriber transcribe --help" for full flag reference.
`, dependencyStatus("ffmpeg"), dependencyStatus("ffprobe"), resolvedConfigPath, fileStatus(resolvedConfigPath), configSource, loadedConfig.Transcription.Model, loadedConfig.Transcription.Format, loadedConfig.Transcription.ChunkingMode, loadedConfig.Transcription.VADMode, loadedConfig.Events.Mode)),
		Example: strings.TrimSpace(`
  export OPENAI_API_KEY="sk-..."
  transcriber doctor
  transcriber probe input.m4a
  transcriber transcribe input.m4a
  transcriber transcribe input.m4a --format json --stdout --events none
`),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.cfgPath)
			if err != nil {
				return domain.NewError("config_load_failed", "failed to load config", domain.ExitConfig, err)
			}
			state.cfg = cfg
			return nil
		},
	}
	root.PersistentFlags().StringVar(&state.cfgPath, "config", "", "config file path")
	root.AddCommand(newTranscribeCmd(state), newProbeCmd(state), newDoctorCmd(state), newVersionCmd(), newConfigCmd(state))
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root
}

func newTranscribeCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcribe <input>",
		Short: "Transcribe an audio or video file",
		Long: strings.TrimSpace(fmt.Sprintf(`
Transcribe an input file and either write artifacts to disk or return the transcript on stdout.

Output modes:
- by default the command writes transcript artifacts to disk
- with --stdout it prints only the transcript payload to stdout
- with --events jsonl it writes progress events to stdout as JSONL

Automation notes:
- --stdout and --events jsonl cannot be used together
- dry-run performs planning only; it does not call the provider or write files
- partial-output controls what happens when a job fails partway through
- if the API key is missing, the command fails early with a missing_api_key error and tells you which environment variable to set

Supported input extensions:
- audio: %s
- video: %s
`, strings.Join(domain.SupportedAudioExtensions(), ", "), strings.Join(domain.SupportedVideoExtensions(), ", "))),
		Example: strings.TrimSpace(`
  transcriber doctor
  transcriber probe input.m4a
  transcriber transcribe input.m4a
  transcriber transcribe input.m4a --format md --out-dir ./out
  transcriber transcribe input.m4a --model gpt-4o-mini-transcribe --language auto
  transcriber transcribe input.m4a --model whisper-1-ts --format json
  transcriber transcribe input.m4a --start 30 --end 90
  transcriber transcribe input.m4a --stdout --format txt --events none
  transcriber transcribe input.m4a --dry-run --events none
  transcriber transcribe input.m4a --events jsonl > events.jsonl
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := buildSpec(state.cfg, cmd, args[0])
			if err != nil {
				return err
			}
			keyEnv := valueOr(spec.APIKeyEnv, "OPENAI_API_KEY")
			if !spec.DryRun && os.Getenv(keyEnv) == "" {
				message := fmt.Sprintf("%s is required. Export it first, then re-run the command. Example: export %s=\"sk-...\"", keyEnv, keyEnv)
				return domain.NewError("missing_api_key", message, domain.ExitAuth, nil)
			}
			state.spec = spec
			services, ctx, cancel, err := buildServices(spec)
			if err != nil {
				return err
			}
			defer cancel()

			artifacts, stdoutData, err := services.Transcribe(ctx, spec)
			if stdoutData != nil {
				_, _ = cmd.OutOrStdout().Write(stdoutData)
				if len(stdoutData) > 0 && stdoutData[len(stdoutData)-1] != '\n' {
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
			} else if spec.EventsMode == domain.EventsText && len(artifacts) > 0 {
				for _, artifact := range artifacts {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), artifact.Path)
				}
			}
			return err
		},
	}
	addCommonTranscribeFlags(cmd)
	return cmd
}

func dependencyStatus(bin string) string {
	if _, err := exec.LookPath(bin); err == nil {
		return "found"
	}
	return "not found"
}

func resolveConfigPath(explicit string) (string, string) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, "flag"
	}
	if value := strings.TrimSpace(os.Getenv("TRANSCRIBER_CONFIG")); value != "" {
		return value, "env"
	}
	return config.DefaultPath(), "default"
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func fileStatus(path string) string {
	if pathExists(path) {
		return "found"
	}
	return "not found"
}

func boolCode(ok bool, okCode, failCode string) string {
	if ok {
		return okCode
	}
	return failCode
}

func newProbeCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe <input>",
		Short: "Probe input media and return execution plan metadata",
		Long: strings.TrimSpace(`
Inspect an input file and return the planned execution metadata as JSON.

Probe does not upload audio to the provider. Use it to check input format,
chunking strategy, timestamp capability, diarize capability, and whether ffmpeg is required.

For GUIs and AI wrappers, it is safest to call probe before starting a real transcription run.
`),
		Example: strings.TrimSpace(`
  transcriber probe input.m4a
  transcriber probe input.m4a --model whisper-1 --format srt
  transcriber probe input.m4a --model gpt-4o-transcribe-diarize --chunking-mode auto
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := buildSpec(state.cfg, cmd, args[0])
			if err != nil {
				return err
			}
			services, ctx, cancel, err := buildServices(spec)
			if err != nil {
				return err
			}
			defer cancel()

			probe, execPlan, err := services.Probe(ctx, spec)
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(map[string]any{"probe": probe, "plan": execPlan}, "", "  ")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	addCommonTranscribeFlags(cmd)
	return cmd
}

func newDoctorCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run environment diagnostics",
		Long: strings.TrimSpace(`
Return environment diagnostics as JSON.

Checks include:
- API key resolution
- ffmpeg / ffprobe resolution
- temporary directory writability
- provider connectivity
- config validity

Doctor performs a connectivity check against the configured provider when credentials are available.
`),
		Example: strings.TrimSpace(`
  transcriber doctor
  transcriber doctor --config ~/.config/transcriber/config.toml
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := buildSpec(state.cfg, cmd, "")
			if err != nil {
				return err
			}
			services, ctx, cancel, err := buildServices(spec)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := services.Doctor(ctx, spec, os.Getenv(state.cfg.API.KeyEnv))
			if err != nil {
				return err
			}
			resolvedPath, source := resolveConfigPath(state.cfgPath)
			exists := pathExists(resolvedPath)
			result.Checks = append(result.Checks,
				domain.DoctorCheck{
					Name:    "config_path",
					OK:      exists,
					Message: fmt.Sprintf("%s (%s)", resolvedPath, source),
					Code:    boolCode(exists, "", "config_not_found"),
				},
				domain.DoctorCheck{
					Name:    "api_key_env",
					OK:      true,
					Message: valueOr(state.cfg.API.KeyEnv, "OPENAI_API_KEY"),
				},
			)
			if cfgErrs := config.ValidateConfig(state.cfg); len(cfgErrs) > 0 {
				result.Checks = append(result.Checks, domain.DoctorCheck{
					Name:    "config",
					OK:      false,
					Message: strings.Join(errorStrings(cfgErrs), "; "),
					Code:    "config_invalid",
				})
			} else {
				result.Checks = append(result.Checks, domain.DoctorCheck{
					Name:    "config",
					OK:      true,
					Message: "config is valid",
				})
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	var jsonOutput bool
	var shortOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version metadata",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Current()
			payload := map[string]string{
				"cli_version":      info.Version,
				"protocol_version": domain.ProtocolVersion,
				"go_module":        "ai-transcriber-cli",
				"openai_sdk":       valueOr(info.SDKVersion, sdkVersion()),
				"build_commit":     info.Commit,
				"build_date":       info.Date,
				"go_version":       info.GoVersion,
			}
			if jsonOutput {
				data, _ := json.MarshalIndent(payload, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			if shortOutput {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), info.Version)
				return nil
			}
			if info.Date != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (%s, %s)\n", info.Version, info.Commit, info.Date)
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", info.Version, info.Commit)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print version metadata as JSON")
	cmd.Flags().BoolVar(&shortOutput, "short", false, "print version only")
	return cmd
}

func newConfigCmd(state *cliState) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Config helpers",
		Long:  "Generate config templates and validate configuration files.",
	}
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Print sample config TOML",
		Long: strings.TrimSpace(`
Write a sample config.toml to stdout or to a target file.

The sample explains that API keys should come from environment variables rather than the config file,
and that audio data is sent to the configured provider during transcription.
`),
		Example: strings.TrimSpace(`
  transcriber config init
  transcriber config init --out ~/.config/transcriber/config.toml
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			outPath, _ := cmd.Flags().GetString("out")
			sample := config.InitSample()
			if outPath == "" {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), sample)
				return nil
			}
			return os.WriteFile(outPath, []byte(sample), 0o644)
		},
	}
	initCmd.Flags().String("out", "", "write sample config to a file")

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate config and optional dictionary",
		Long: strings.TrimSpace(`
Validate the config file and optional dictionary, then return the result as JSON.

Validation checks include model / format compatibility, prompt restrictions,
and dependency resolution such as ffmpeg / ffprobe availability.
This is useful as a sanity check before execution in CI or AI wrappers.
`),
		Example: strings.TrimSpace(`
  transcriber config validate
  transcriber config validate --config ~/.config/transcriber/config.toml
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.cfgPath)
			if err != nil {
				return err
			}
			errs := config.ValidateConfig(cfg)
			payload := map[string]any{"valid": len(errs) == 0}
			if len(errs) > 0 {
				messages := errorStrings(errs)
				payload["errors"] = messages
				data, _ := json.MarshalIndent(payload, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return domain.NewError("config_invalid", "config validation failed", domain.ExitConfig, errors.New(strings.Join(messages, "; ")))
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	configCmd.AddCommand(initCmd, validateCmd)
	return configCmd
}

func addCommonTranscribeFlags(cmd *cobra.Command) {
	cmd.Flags().String("out", "", "write transcript to this exact output path")
	cmd.Flags().String("out-dir", "", "write transcript artifacts into this directory")
	cmd.Flags().String("format", "", "output format: txt, md, json, srt, vtt")
	cmd.Flags().Bool("stdout", false, "write transcript payload to stdout instead of files")
	cmd.Flags().Bool("overwrite", false, "overwrite existing transcript, manifest, and raw JSON outputs")
	cmd.Flags().Bool("write-manifest", true, "write manifest JSON alongside the transcript output")
	cmd.Flags().String("raw-provider-json", "", "write raw provider response JSON to this path")
	cmd.Flags().String("model", "", "transcription model: gpt-4o-transcribe, gpt-4o-mini-transcribe, whisper-1, whisper-1-ts, gpt-4o-transcribe-diarize")
	cmd.Flags().String("language", "", "language hint such as auto, ja, en; prefer auto for mixed-language audio")
	cmd.Flags().String("prompt", "", "provider prompt for transcription models that support prompts")
	cmd.Flags().Bool("logprobs", false, "include logprobs when supported by the selected model")
	cmd.Flags().Bool("diarize", false, "shorthand for --model gpt-4o-transcribe-diarize")
	cmd.Flags().String("chunking-mode", "", "chunking mode: auto, off, server-auto, server-vad, client")
	cmd.Flags().String("vad-mode", "", "VAD mode: disabled, local")
	cmd.Flags().Float64("chunk-target-sec", 0, "advanced override for client chunk target length in seconds")
	cmd.Flags().Float64("chunk-overlap-sec", 0, "advanced override for client chunk overlap in seconds")
	cmd.Flags().Float64("server-vad-threshold", 0.0, "server VAD threshold override")
	cmd.Flags().Int("server-vad-prefix-ms", 0, "server VAD prefix padding override in milliseconds")
	cmd.Flags().Int("server-vad-silence-ms", 0, "server VAD silence duration override in milliseconds")
	cmd.Flags().String("obsidian-vad-mode", "", "legacy compatibility alias for older Obsidian workflows")
	cmd.Flags().String("dictionary", "", "dictionary YAML path")
	cmd.Flags().Bool("dictionary-enabled", false, "enable deterministic dictionary corrections")
	cmd.Flags().Bool("postprocess", false, "enable transcript-level AI postprocess")
	cmd.Flags().String("postprocess-model", "", "postprocess model override; defaults to gpt-4o-mini")
	cmd.Flags().String("postprocess-prompt", "", "extra postprocess instructions appended to the safe default prompt")
	cmd.Flags().String("start", "", "trim start offset in seconds or HH:MM:SS form")
	cmd.Flags().String("end", "", "trim end offset in seconds or HH:MM:SS form")
	cmd.Flags().String("events", "", "events mode: text, jsonl, none")
	cmd.Flags().Bool("quiet", false, "reduce logs to warnings and errors")
	cmd.Flags().Bool("verbose", false, "enable debug logging")
	cmd.Flags().String("log-format", "", "log format: text or json")
	cmd.Flags().String("job-id", "", "explicit job ID for manifests and JSONL events")
	cmd.Flags().String("ffmpeg", "", "ffmpeg executable path")
	cmd.Flags().String("ffprobe", "", "ffprobe executable path")
	cmd.Flags().Bool("keep-workdir", false, "keep the temporary workdir and uploaded intermediate .m4a artifacts after completion")
	cmd.Flags().String("workdir", "", "temporary workdir path for normalized and chunked .m4a artifacts")
	cmd.Flags().Bool("resume", false, "resume a prior client-chunked transcription using the existing manifest and chunk cache")
	cmd.Flags().Bool("parallel", false, "send client chunks in parallel for gpt-4o-transcribe and gpt-4o-mini-transcribe")
	cmd.Flags().Duration("timeout", 0, "overall job timeout, for example 90s or 10m")
	cmd.Flags().Int("retries", 1, "retry count for retryable provider failures")
	cmd.Flags().String("partial-output", "", "partial output policy on failure: write, discard, stdout")
	cmd.Flags().Bool("dry-run", false, "build the execution plan and exit without calling the provider")
	cmd.Flags().Bool("include-segments", false, "include segment list in markdown output")
	cmd.Flags().Bool("allow-experimental-diarize-stitching", false, "allow experimental long-form diarize chunk stitching")
	cmd.Flags().StringArray("speaker-ref", nil, "speaker reference in name=path form; each clip should be about 2-10 seconds")
}

func buildSpec(cfg config.AppConfig, cmd *cobra.Command, input string) (domain.JobSpec, error) {
	if input != "" {
		abs, err := filepath.Abs(input)
		if err != nil {
			return domain.JobSpec{}, domain.NewError("input_path_invalid", "failed to resolve input path", domain.ExitInput, err)
		}
		input = abs
	}
	if _, err := logging.ParseLevel(getBool(cmd, "quiet"), getBool(cmd, "verbose")); err != nil {
		return domain.JobSpec{}, domain.NewError("log_flags_conflict", err.Error(), domain.ExitArgs, err)
	}

	speakerRefs, err := parseSpeakerRefs(getStringArray(cmd, "speaker-ref"))
	if err != nil {
		return domain.JobSpec{}, err
	}

	spec := domain.JobSpec{
		JobID:                             valueOr(getString(cmd, "job-id"), app.GenerateJobID()),
		APIKeyEnv:                         valueOr(cfg.API.KeyEnv, "OPENAI_API_KEY"),
		InputPath:                         input,
		OutputPath:                        getString(cmd, "out"),
		OutputDir:                         getString(cmd, "out-dir"),
		Format:                            parseFormat(valueOr(getString(cmd, "format"), string(cfg.Transcription.Format))),
		Model:                             valueOr(getString(cmd, "model"), cfg.Transcription.Model),
		Language:                          valueOr(getString(cmd, "language"), cfg.Transcription.Language),
		Prompt:                            valueOr(getString(cmd, "prompt"), cfg.Transcription.Prompt),
		Logprobs:                          boolValue(cmd, "logprobs", cfg.Transcription.Logprobs),
		ChunkingMode:                      parseChunking(valueOr(getString(cmd, "chunking-mode"), string(cfg.Transcription.ChunkingMode))),
		VADMode:                           parseVAD(valueOr(getString(cmd, "vad-mode"), string(cfg.Transcription.VADMode))),
		DictionaryPath:                    valueOr(getString(cmd, "dictionary"), cfg.Dictionary.Path),
		DictionaryEnabled:                 boolValue(cmd, "dictionary-enabled", cfg.Dictionary.Enabled),
		Postprocess:                       boolValue(cmd, "postprocess", cfg.Postprocess.Enabled),
		PostprocessModel:                  valueOr(getString(cmd, "postprocess-model"), cfg.Postprocess.Model),
		PostprocessPrompt:                 valueOr(getString(cmd, "postprocess-prompt"), cfg.Postprocess.Prompt),
		EventsMode:                        parseEvents(valueOr(getString(cmd, "events"), string(cfg.Events.Mode))),
		LogFormat:                         parseLogFormat(valueOr(getString(cmd, "log-format"), string(cfg.Events.LogFormat))),
		Quiet:                             getBool(cmd, "quiet"),
		Verbose:                           getBool(cmd, "verbose"),
		FFmpegPath:                        valueOr(getString(cmd, "ffmpeg"), envOr("TRANSCRIBER_FFMPEG", cfg.Paths.FFmpeg)),
		FFprobePath:                       valueOr(getString(cmd, "ffprobe"), envOr("TRANSCRIBER_FFPROBE", cfg.Paths.FFprobe)),
		KeepWorkdir:                       boolValue(cmd, "keep-workdir", cfg.Paths.KeepWorkdir),
		Workdir:                           valueOr(getString(cmd, "workdir"), cfg.Paths.Workdir),
		Resume:                            getBool(cmd, "resume"),
		Timeout:                           getDuration(cmd, "timeout"),
		Retries:                           getInt(cmd, "retries"),
		Parallel:                          getBool(cmd, "parallel"),
		PartialOutput:                     parsePartial(valueOr(getString(cmd, "partial-output"), string(cfg.Output.PartialOutput))),
		Overwrite:                         boolValue(cmd, "overwrite", cfg.Output.Overwrite),
		WriteManifest:                     boolValue(cmd, "write-manifest", cfg.Output.WriteManifest),
		RawProviderJSONPath:               getString(cmd, "raw-provider-json"),
		Stdout:                            getBool(cmd, "stdout"),
		DryRun:                            getBool(cmd, "dry-run"),
		IncludeSegments:                   getBool(cmd, "include-segments"),
		AllowExperimentalDiarizeStitching: getBool(cmd, "allow-experimental-diarize-stitching"),
		ServerVADThreshold:                firstFloat(getFloat(cmd, "server-vad-threshold"), cfg.ServerVAD.Threshold),
		ServerVADPrefixMS:                 firstInt(getInt(cmd, "server-vad-prefix-ms"), cfg.ServerVAD.PrefixPaddingMS),
		ServerVADSilenceMS:                firstInt(getInt(cmd, "server-vad-silence-ms"), cfg.ServerVAD.SilenceDurationMS),
		SpeakerRefs:                       speakerRefs,
	}
	if cmd.Flags().Changed("chunk-target-sec") {
		value := getFloat(cmd, "chunk-target-sec")
		spec.ChunkTargetSecOverride = &value
	}
	if cmd.Flags().Changed("chunk-overlap-sec") {
		value := getFloat(cmd, "chunk-overlap-sec")
		spec.ChunkOverlapSecOverride = &value
	}
	if obsidianMode := getString(cmd, "obsidian-vad-mode"); obsidianMode != "" {
		switch obsidianMode {
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
	if getBool(cmd, "diarize") {
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

	if spec.StartSec, err = parseOptionalTime(getString(cmd, "start")); err != nil {
		return domain.JobSpec{}, domain.NewError("start_invalid", "invalid --start value", domain.ExitArgs, err)
	}
	if spec.EndSec, err = parseOptionalTime(getString(cmd, "end")); err != nil {
		return domain.JobSpec{}, domain.NewError("end_invalid", "invalid --end value", domain.ExitArgs, err)
	}
	if spec.StartSec != nil && spec.EndSec != nil && *spec.EndSec <= *spec.StartSec {
		return domain.JobSpec{}, domain.NewError("time_range_invalid", "--end must be greater than --start", domain.ExitArgs, nil)
	}
	return spec, nil
}

func supportsManualParallel(model string) bool {
	switch model {
	case "gpt-4o-transcribe", "gpt-4o-mini-transcribe":
		return true
	default:
		return false
	}
}

func buildServices(spec domain.JobSpec) (*app.Services, context.Context, context.CancelFunc, error) {
	level, err := logging.ParseLevel(spec.Quiet, spec.Verbose)
	if err != nil {
		return nil, nil, nil, err
	}
	if !spec.Quiet && !spec.Verbose {
		level = parseLogLevelEnv(level)
	}

	logger := logging.New(os.Stderr, level, spec.LogFormat)
	var eventWriter events.Writer
	switch spec.EventsMode {
	case domain.EventsJSONL:
		eventWriter = events.NewJSONLWriter(os.Stdout, spec.JobID)
	case domain.EventsText:
		eventWriter = events.NewTextWriter(os.Stderr)
	default:
		eventWriter = events.NopWriter{}
	}

	baseCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, timeoutCancel := openaip.MustTimeout(baseCtx, spec.Timeout)
	secondSignal := make(chan os.Signal, 1)
	signal.Notify(secondSignal, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	var interruptCount atomic.Int32
	go func() {
		for {
			select {
			case <-done:
				return
			case <-secondSignal:
				if interruptCount.Add(1) > 1 {
					os.Exit(domain.ExitCancelled)
				}
			}
		}
	}()

	cancel := func() {
		close(done)
		signal.Stop(secondSignal)
		timeoutCancel()
		stopSignals()
	}

	apiKey := os.Getenv(valueOr(spec.APIKeyEnv, "OPENAI_API_KEY"))
	var provider app.Provider
	if strings.TrimSpace(apiKey) != "" {
		provider = openaip.New(apiKey)
	}
	services := app.New(media.NewService(), plan.NewPlanner(), provider, vad.NewService(), postprocess.NewService(apiKey), logger, eventWriter)
	return services, ctx, cancel, nil
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

func parseFormat(v string) domain.OutputFormat       { return domain.OutputFormat(v) }
func parseChunking(v string) domain.ChunkingMode     { return domain.ChunkingMode(v) }
func parseVAD(v string) domain.VADMode               { return domain.VADMode(v) }
func parseEvents(v string) domain.EventsMode         { return domain.EventsMode(v) }
func parseLogFormat(v string) domain.LogFormat       { return domain.LogFormat(v) }
func parsePartial(v string) domain.PartialOutputMode { return domain.PartialOutputMode(v) }

func parseSpeakerRefs(values []string) ([]domain.SpeakerReference, error) {
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
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, domain.NewError("speaker_ref_invalid", "failed to resolve speaker reference path", domain.ExitArgs, err)
		}
		// Resolve speaker references at the CLI boundary so provider validation and
		// manifest data never have to guess which file the user intended.
		refs = append(refs, domain.SpeakerReference{Name: name, Path: absPath})
	}
	return refs, nil
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func boolValue(cmd *cobra.Command, name string, fallback bool) bool {
	if cmd.Flags().Changed(name) {
		return getBool(cmd, name)
	}
	return fallback
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

func getString(cmd *cobra.Command, name string) string { v, _ := cmd.Flags().GetString(name); return v }
func getStringArray(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringArray(name)
	return v
}
func getBool(cmd *cobra.Command, name string) bool { v, _ := cmd.Flags().GetBool(name); return v }
func getInt(cmd *cobra.Command, name string) int   { v, _ := cmd.Flags().GetInt(name); return v }
func getFloat(cmd *cobra.Command, name string) float64 {
	v, _ := cmd.Flags().GetFloat64(name)
	return v
}
func getDuration(cmd *cobra.Command, name string) time.Duration {
	v, _ := cmd.Flags().GetDuration(name)
	return v
}

func parseLogLevelEnv(fallback logging.Level) logging.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRANSCRIBER_LOG_LEVEL"))) {
	case "error":
		return logging.LevelError
	case "warn", "warning":
		return logging.LevelWarn
	case "debug":
		return logging.LevelDebug
	case "info":
		return logging.LevelInfo
	default:
		return fallback
	}
}

func sdkVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/openai/openai-go/v3" {
			return dep.Version
		}
	}
	return ""
}

func errorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}
