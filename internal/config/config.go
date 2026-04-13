package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"ai-transcriber-cli/internal/dictionary"
	"ai-transcriber-cli/internal/domain"
)

type APIConfig struct {
	KeyEnv string `toml:"key_env"`
}

type TranscriptionConfig struct {
	Model        string              `toml:"model"`
	Language     string              `toml:"language"`
	Format       domain.OutputFormat `toml:"format"`
	ChunkingMode domain.ChunkingMode `toml:"chunking_mode"`
	VADMode      domain.VADMode      `toml:"vad_mode"`
	Prompt       string              `toml:"prompt"`
	Logprobs     bool                `toml:"logprobs"`
}

type ServerVADConfig struct {
	Threshold         float64 `toml:"threshold"`
	PrefixPaddingMS   int     `toml:"prefix_padding_ms"`
	SilenceDurationMS int     `toml:"silence_duration_ms"`
}

type PostprocessConfig struct {
	Enabled bool   `toml:"enabled"`
	Model   string `toml:"model"`
	Prompt  string `toml:"prompt"`
}

type DictionaryConfig struct {
	Enabled bool   `toml:"enabled"`
	Path    string `toml:"path"`
}

type PathsConfig struct {
	FFmpeg      string `toml:"ffmpeg"`
	FFprobe     string `toml:"ffprobe"`
	Workdir     string `toml:"workdir"`
	KeepWorkdir bool   `toml:"keep_workdir"`
}

type OutputConfig struct {
	Overwrite     bool                     `toml:"overwrite"`
	WriteManifest bool                     `toml:"write_manifest"`
	PartialOutput domain.PartialOutputMode `toml:"partial_output"`
}

type EventsConfig struct {
	Mode      domain.EventsMode `toml:"mode"`
	LogFormat domain.LogFormat  `toml:"log_format"`
}

type AppConfig struct {
	API           APIConfig           `toml:"api"`
	Transcription TranscriptionConfig `toml:"transcription"`
	ServerVAD     ServerVADConfig     `toml:"server_vad"`
	Postprocess   PostprocessConfig   `toml:"postprocess"`
	Dictionary    DictionaryConfig    `toml:"dictionary"`
	Paths         PathsConfig         `toml:"paths"`
	Output        OutputConfig        `toml:"output"`
	Events        EventsConfig        `toml:"events"`
}

func Default() AppConfig {
	return AppConfig{
		API: APIConfig{KeyEnv: "OPENAI_API_KEY"},
		Transcription: TranscriptionConfig{
			Model:        "gpt-4o-transcribe",
			Language:     "auto",
			Format:       domain.FormatMD,
			ChunkingMode: domain.ChunkingAuto,
			VADMode:      domain.VADDisabled,
			Prompt:       "",
			Logprobs:     false,
		},
		ServerVAD: ServerVADConfig{
			Threshold:         0.5,
			PrefixPaddingMS:   300,
			SilenceDurationMS: 800,
		},
		Postprocess: PostprocessConfig{
			Model: "gpt-4o-mini",
		},
		Dictionary: DictionaryConfig{},
		Paths: PathsConfig{
			FFmpeg:  "ffmpeg",
			FFprobe: "ffprobe",
		},
		Output: OutputConfig{
			Overwrite:     false,
			WriteManifest: true,
			PartialOutput: domain.PartialWrite,
		},
		Events: EventsConfig{
			Mode:      domain.EventsText,
			LogFormat: domain.LogFormatText,
		},
	}
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		appData := os.Getenv("AppData")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "transcriber", "config.toml")
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "transcriber", "config.toml")
}

func Load(path string) (AppConfig, error) {
	cfg := Default()
	if path == "" {
		path = os.Getenv("TRANSCRIBER_CONFIG")
	}
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func InitSample() string {
	return strings.TrimSpace(`
# Audio is sent to the configured provider during transcription and doctor connectivity checks.
[api]
key_env = "OPENAI_API_KEY"

[transcription]
# gpt-4o-transcribe is a good default when you want the output to preserve
# mixed-language speech as spoken. Prefer whisper-1 when timestamps or a more
# aggressively normalized single-language transcript matter more.
model = "gpt-4o-transcribe"
# Keep auto unless you are confident the source is predominantly one language.
# For code-switched audio, forcing a single language can improve readability
# while dropping words or phrases from the other language.
language = "auto"
format = "md"
chunking_mode = "auto"
vad_mode = "disabled"
prompt = ""
logprobs = false

[server_vad]
threshold = 0.5
prefix_padding_ms = 300
silence_duration_ms = 800

[postprocess]
enabled = false
model = "gpt-4o-mini"
prompt = ""

[dictionary]
enabled = false
path = ""

[paths]
ffmpeg = "ffmpeg"
ffprobe = "ffprobe"
workdir = ""
keep_workdir = false

[output]
overwrite = false
write_manifest = true
partial_output = "write"

[events]
mode = "text"
log_format = "text"
`) + "\n"
}

func ValidateConfig(cfg AppConfig) []error {
	var errs []error
	if cfg.Transcription.Model == "" {
		errs = append(errs, fmt.Errorf("transcription.model is required"))
	}
	appendValidationError(&errs, domain.ValidateModel(cfg.Transcription.Model))
	switch cfg.Transcription.Format {
	case domain.FormatTXT, domain.FormatMD, domain.FormatJSON, domain.FormatSRT, domain.FormatVTT:
	default:
		errs = append(errs, fmt.Errorf("unsupported transcription.format: %s", cfg.Transcription.Format))
	}
	appendValidationError(&errs, domain.ValidateSubtitleFormat(cfg.Transcription.Model, cfg.Transcription.Format))
	appendValidationError(&errs, domain.ValidatePrompt(cfg.Transcription.Model, strings.TrimSpace(cfg.Transcription.Prompt)))
	appendValidationError(&errs, domain.ValidateLogprobs(cfg.Transcription.Model, cfg.Transcription.Logprobs))
	if cfg.Dictionary.Enabled {
		if strings.TrimSpace(cfg.Dictionary.Path) == "" {
			errs = append(errs, fmt.Errorf("dictionary.path is required when dictionary.enabled is true"))
		} else if _, err := os.Stat(cfg.Dictionary.Path); err != nil {
			errs = append(errs, fmt.Errorf("dictionary.path does not exist: %s", cfg.Dictionary.Path))
		} else if _, err := dictionary.Load(cfg.Dictionary.Path); err != nil {
			errs = append(errs, fmt.Errorf("dictionary.path is invalid: %v", err))
		}
	}
	if needsFFmpeg(cfg) {
		if !resolvable(cfg.Paths.FFmpeg, "ffmpeg") {
			errs = append(errs, fmt.Errorf("ffmpeg is required but not resolvable"))
		}
		if !resolvable(cfg.Paths.FFprobe, "ffprobe") {
			errs = append(errs, fmt.Errorf("ffprobe is required but not resolvable"))
		}
	}
	return errs
}

func needsFFmpeg(cfg AppConfig) bool {
	return cfg.Transcription.ChunkingMode == domain.ChunkingClient || cfg.Transcription.VADMode == domain.VADLocal
}

func resolvable(pathValue, fallback string) bool {
	candidate := strings.TrimSpace(pathValue)
	if candidate == "" {
		candidate = fallback
	}
	if strings.Contains(candidate, string(filepath.Separator)) {
		_, err := os.Stat(candidate)
		return err == nil
	}
	_, err := execLookPath(candidate)
	return err == nil
}

var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

func appendValidationError(dst *[]error, err error) {
	if err != nil {
		*dst = append(*dst, err)
	}
}
