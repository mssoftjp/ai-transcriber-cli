# Config Reference

Audience: operators, advanced CLI users, and integrators who need the full configuration surface.

This document is the complete reference for `config.toml`, related environment variables, and the most important CLI-only overrides.

## Core Rules

- config format: `TOML`
- precedence: `flags > env > explicit config > default config > internal defaults`
- API keys are not stored in `config.toml`
- the default API key environment variable name is `OPENAI_API_KEY`
- `transcriber config init` prints a current sample config
- `transcriber config validate` checks a config file against runtime rules

## Default Config Path

- macOS / Linux: `~/.config/transcriber/config.toml`
- Windows: `%AppData%/transcriber/config.toml`

You can override the path with:

- `--config /abs/path/to/config.toml`
- `TRANSCRIBER_CONFIG=/abs/path/to/config.toml`

## Environment Variables

These variables are read by the binary directly:

- `OPENAI_API_KEY`
  - default API key source
  - actual variable name can be changed through `[api].key_env`
- `TRANSCRIBER_CONFIG`
  - config file path override
- `TRANSCRIBER_FFMPEG`
  - fallback override for the ffmpeg executable path
- `TRANSCRIBER_FFPROBE`
  - fallback override for the ffprobe executable path
- `TRANSCRIBER_LOG_LEVEL`
  - log level override used by the CLI runtime

## Full `config.toml` Sample

This sample matches `transcriber config init`.

```toml
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
```

## Config File Keys

Every supported config key is listed below with its default.

### `[api]`

#### `key_env`

- type: `string`
- default: `"OPENAI_API_KEY"`
- meaning: environment variable name used to read the API key at runtime

## `[transcription]`

### `model`

- type: `string`
- default: `"gpt-4o-transcribe"`
- allowed values:
  - `gpt-4o-transcribe`
  - `gpt-4o-mini-transcribe`
  - `whisper-1`
  - `whisper-1-ts`
  - `gpt-4o-transcribe-diarize`
- notes:
  - `gpt-4o-transcribe` is a strong default when mixed-language fidelity matters
  - `gpt-4o-mini-transcribe` is often worth comparing directly against `gpt-4o-transcribe`
  - `whisper-1` and `whisper-1-ts` are strong choices when timestamps matter
  - `gpt-4o-transcribe-diarize` is for speaker-labeled output

### `language`

- type: `string`
- default: `"auto"`
- meaning:
  - use `"auto"` to let the model infer language
  - use a language code such as `ja` or `en` when you want to steer recognition toward a dominant language
- note:
  - `auto` is usually the safest starting point for mixed-language audio

### `format`

- type: `string`
- default: `"md"`
- allowed values:
  - `txt`
  - `md`
  - `json`
  - `srt`
  - `vtt`

### `chunking_mode`

- type: `string`
- default: `"auto"`
- allowed values:
  - `auto`
  - `off`
  - `server-auto`
  - `server-vad`
  - `client`
- meaning:
  - `auto`: planner chooses
  - `off`: single request only
  - `server-auto`: provider-side chunking when supported
  - `server-vad`: provider-side VAD chunking when supported
  - `client`: local chunking before upload

### `vad_mode`

- type: `string`
- default: `"disabled"`
- allowed values:
  - `disabled`
  - `local`
- meaning:
  - `local` enables local chunk-boundary optimization for client chunking paths

### `prompt`

- type: `string`
- default: `""`
- meaning:
  - optional transcription prompt sent to supported models
- note:
  - diarize requests do not support `prompt`

### `logprobs`

- type: `bool`
- default: `false`
- meaning:
  - request logprobs when supported by the selected model
- note:
  - supported only by `gpt-4o-transcribe` and `gpt-4o-mini-transcribe`

## `[server_vad]`

### `threshold`

- type: `float`
- default: `0.5`
- meaning:
  - server-side VAD sensitivity threshold when `chunking_mode = "server-vad"`

### `prefix_padding_ms`

- type: `int`
- default: `300`
- meaning:
  - prefix padding in milliseconds for server-side VAD chunking

### `silence_duration_ms`

- type: `int`
- default: `800`
- meaning:
  - silence duration in milliseconds for server-side VAD chunking

## `[postprocess]`

### `enabled`

- type: `bool`
- default: `false`
- meaning:
  - enables transcript-level postprocess after transcription

### `model`

- type: `string`
- default: `"gpt-4o-mini"`
- meaning:
  - model used for AI postprocess when postprocess is enabled

### `prompt`

- type: `string`
- default: `""`
- meaning:
  - additional transcript-level postprocess instructions

## `[dictionary]`

### `enabled`

- type: `bool`
- default: `false`
- meaning:
  - enables deterministic dictionary correction

### `path`

- type: `string`
- default: `""`
- meaning:
  - path to a dictionary YAML file

## `[paths]`

### `ffmpeg`

- type: `string`
- default: `"ffmpeg"`
- meaning:
  - ffmpeg executable path or command name

### `ffprobe`

- type: `string`
- default: `"ffprobe"`
- meaning:
  - ffprobe executable path or command name

### `workdir`

- type: `string`
- default: `""`
- meaning:
  - explicit working directory for normalized audio and chunk artifacts
  - retained artifacts are provider-ready `.m4a` files when ffmpeg preprocessing is used
  - empty means the OS temp directory is used
  - resume does not depend on reusing this directory; resumable chunk caches live next to the transcript artifacts

### `keep_workdir`

- type: `bool`
- default: `false`
- meaning:
  - keeps the working directory after the job completes
  - useful when you want to inspect the exact `.m4a` files that were uploaded

## `[output]`

### `overwrite`

- type: `bool`
- default: `false`
- meaning:
  - allows overwriting existing transcript, manifest, and raw provider JSON outputs
- note:
  - emergency partial output may still overwrite its own partial artifact path so a failed run can preserve the latest partial result

### `write_manifest`

- type: `bool`
- default: `true`
- meaning:
  - write a manifest JSON file next to the transcript artifact

### `partial_output`

- type: `string`
- default: `"write"`
- allowed values:
  - `write`
  - `discard`
  - `stdout`
- meaning:
  - controls what happens to partial results after failure or cancellation

## `[events]`

### `mode`

- type: `string`
- default: `"text"`
- allowed values:
  - `text`
  - `jsonl`
  - `none`
- meaning:
  - controls progress/event output mode

### `log_format`

- type: `string`
- default: `"text"`
- allowed values:
  - `text`
  - `json`
- meaning:
  - controls log formatting

## Validation Rules

These rules are enforced by `transcriber config validate` and by runtime planning:

- `transcription.model` must be supported
- `transcription.format` must be supported
- `srt` and `vtt` require a timestamp-capable model
- `gpt-4o-transcribe-diarize` does not support `prompt`
- `logprobs` is supported only by `gpt-4o-transcribe` and `gpt-4o-mini-transcribe`
- if `dictionary.enabled = true`, `dictionary.path` must be set and valid
- if the config implies client chunking or local VAD, `ffmpeg` and `ffprobe` must be resolvable
- long-form diarize stitching requires the CLI flag `--allow-experimental-diarize-stitching`

## CLI-Only Flags

These controls are not stored in `config.toml`, but they are part of normal operation and override config values when present:

- `--config`
- `--stdout`
- `--events`
- `--dry-run`
- `--raw-provider-json`
- `--partial-output`
- `--out`
- `--out-dir`
- `--write-manifest`
- `--overwrite`
- `--start`
- `--end`
- `--timeout`
- `--retries`
- `--ffmpeg`
- `--ffprobe`
- `--workdir`
- `--keep-workdir`
- `--resume`
- `--parallel`
- `--job-id`
- `--speaker-ref`
- `--allow-experimental-diarize-stitching`

### `--parallel`

- CLI-only; not stored in `config.toml`
- supported only by `gpt-4o-transcribe` and `gpt-4o-mini-transcribe`
- only affects runs that actually use `client` chunking
- when effective, chunk requests are sent concurrently and prompt carryover is disabled
- if the planner chooses single-request or server-side chunking, the flag has no effect and the job proceeds normally
- if you later run `--resume`, the resumable manifest must match the original chunk execution mode

## Time Range Selection

Time range selection is intentionally CLI-only.

- `--start` accepts seconds or `HH:MM:SS`
- `--end` accepts seconds or `HH:MM:SS`
- these options commonly require `ffmpeg` and `ffprobe` because the input may need trimming before upload

Examples:

```sh
transcriber transcribe input.m4a --start 30 --end 90
transcriber transcribe input.m4a --start 00:01:30 --end 00:03:00
```

## Recommended Usage Pattern

The intended operational pattern is:

- put stable team or personal defaults in `config.toml`
- override them per run with CLI flags
- keep secrets in environment variables instead of config files

This makes the config file a baseline and the CLI invocation the final source of truth for a specific run.
