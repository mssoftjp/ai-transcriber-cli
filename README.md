# ai-transcriber-cli

A Go CLI that transcribes audio and video files via the OpenAI speech-to-text API and writes results as `txt`, `md`, `json`, `srt`, or `vtt`.

## Installation

### From release archive (recommended, macOS)

```sh
base="https://github.com/mssoftjp/ai-transcriber-cli/releases/latest/download"
curl -L -O "$base/checksums.txt"
archive="$(awk '/darwin_arm64\.tar\.gz$/ {print $2; exit}' checksums.txt)"
curl -L -O "$base/$archive"
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf "$archive"
mkdir -p "$HOME/.local/bin"
install -m 0755 "${archive%.tar.gz}" "$HOME/.local/bin/transcriber"
export PATH="$HOME/.local/bin:$PATH"
transcriber version
```

If you prefer a manual install, download the latest archive from [GitHub Releases](https://github.com/mssoftjp/ai-transcriber-cli/releases), extract it, and place `transcriber` on your `PATH`.

### Install ffmpeg (recommended)

`ffmpeg` and `ffprobe` are needed for video input, long-file chunking, trimming, and format normalization. Small audio files in a provider-compatible format can be transcribed without them.

When `ffmpeg` is used, the CLI now produces provider-ready intermediate audio as compact `.m4a` files rather than large uncompressed WAV files. If you keep the workdir, the files left behind are the same files that were uploaded.

```sh
brew install ffmpeg          # macOS (https://formulae.brew.sh/formula/ffmpeg)
sudo apt install ffmpeg      # Debian / Ubuntu
```

Windows: download from [ffmpeg.org/download.html](https://ffmpeg.org/download.html) and add the `bin` directory to your `PATH`.

### Set your API key

```sh
export OPENAI_API_KEY="sk-..."
```

The CLI reads the API key only from environment variables. It never writes keys to config files, log files, or transcript output. Audio data is sent to the OpenAI API for transcription and is subject to [OpenAI's data usage policies](https://openai.com/policies/usage-policies). No audio or transcript data is sent anywhere else.

### Verify

```sh
transcriber doctor
```

This checks API key visibility, `ffmpeg` / `ffprobe` availability, temp directory access, provider connectivity, and config validity.

### From source

```sh
git clone https://github.com/mssoftjp/ai-transcriber-cli.git
cd ai-transcriber-cli
make install
transcriber version
```

## Basic Usage

```sh
# Simplest form — writes Markdown output next to the input file
transcriber transcribe input.m4a

# Print plain text to stdout
transcriber transcribe input.m4a --format txt --stdout --events none

# Write JSON to a specific directory
transcriber transcribe input.m4a --format json --out-dir ./out
```

By default, the output file is written next to the input as `<name>.transcript.md`. A manifest sidecar (`<name>.transcript.manifest.json`) is also created. Use `--out` or `--out-dir` to change the destination, and `--overwrite` to allow replacing existing output.

Supported input formats include `.mp3`, `.m4a`, `.wav`, `.flac`, `.ogg`, `.mp4`, `.mov`, `.mkv`, and others. Run `transcriber transcribe --help` for the full list. Long files are automatically split into chunks and reassembled.

If a long client-chunked job fails partway through, re-run the same command with `--resume` to reuse completed chunks from the manifest sidecar and chunk cache next to the output artifacts.

## Choosing a Model

| Model | Strengths | Good for |
|-------|-----------|----------|
| `gpt-4o-transcribe` | High accuracy, preserves code-switched audio | General transcription (default) |
| `gpt-4o-mini-transcribe` | Lighter, lower cost | Cost-sensitive workloads |
| `whisper-1` | Timestamp-capable output | Subtitle generation (srt/vtt) |
| `gpt-4o-transcribe-diarize` | Speaker-labeled output | Meetings, multi-speaker recordings |

```sh
# Generate subtitles
transcriber transcribe input.m4a --model whisper-1 --format srt

# Speaker diarization
transcriber transcribe call.m4a --diarize --format json
```

## Language

The default is `auto` (automatic detection).

```sh
# Auto-detect — best for mixed-language audio
transcriber transcribe input.m4a --language auto

# Force a single language — suppresses other-language content
transcriber transcribe input.m4a --language ja
```

For audio that mixes multiple languages, `auto` tends to preserve the original speech more faithfully. Forcing a single language improves readability but may drop content in other languages.

## Common Options

### Time Range

```sh
transcriber transcribe input.m4a --start 30 --end 90
transcriber transcribe input.m4a --start 00:01:30 --end 00:03:00
```

### Dictionary Corrections

A YAML dictionary file can automatically fix common recognition errors.

```sh
transcriber transcribe input.m4a --dictionary ./dict.yaml --dictionary-enabled
```

### AI Postprocess

Runs a transcript-level correction pass after transcription. It does not summarize or translate.

```sh
transcriber transcribe input.m4a --postprocess
```

### Pre-Flight Checks (dry-run / probe)

Inspect the execution plan without calling the API.

```sh
# probe: returns input metadata and the planned strategy as JSON
transcriber probe input.m4a

# dry-run: same entry point as transcribe, but stops after planning
transcriber transcribe input.m4a --dry-run --events none
```

### JSONL Events (GUI / Automation)

```sh
transcriber transcribe input.m4a --events jsonl > events.jsonl
```

Emits machine-readable JSONL progress events to stdout. Designed for GUI wrappers and automation pipelines.

## Configuration

A TOML config file lets you persist frequently used options as defaults.

- macOS / Linux: `~/.config/transcriber/config.toml`
- Windows: `%AppData%/transcriber/config.toml`

```sh
# Generate a sample config
transcriber config init

# Validate the current config
transcriber config validate
```

Precedence: CLI flags > environment variables > config file > built-in defaults

API keys should be kept in environment variables. The CLI does not store, log, or embed API keys in any output. The `--postprocess` option sends transcript text (not audio) to the OpenAI API for correction; this is the only case where transcript content leaves the local machine after the initial transcription call.

See [docs/config.md](docs/config.md) for the full reference.

## Commands

| Command | Description |
|---------|-------------|
| `transcriber transcribe <input>` | Run transcription |
| `transcriber probe <input>` | Inspect input and return the execution plan |
| `transcriber doctor` | Check environment (API key, dependencies) |
| `transcriber version` | Print version metadata |
| `transcriber config init` | Print a sample config |
| `transcriber config validate` | Validate config and dictionary |

Run `transcriber <command> --help` for the full flag reference of each command.

## Documentation

- [docs/contracts.md](docs/contracts.md) — Public contracts for GUI and automation integrations
- [docs/config.md](docs/config.md) — Full config reference for operators and advanced users
- [docs/limitations.md](docs/limitations.md) — Known constraints and tradeoffs for maintainers and adopters

## Development

```sh
make build    # build
make test     # test
make ci       # lint + test + vet
make hooks    # enable Git hooks (once)
```

### Integration Tests

Tests that call the real API are not run by `go test ./...`.

```sh
OPENAI_API_KEY=... go test ./internal/provider/openai ./internal/postprocess -run Integration -count=1
```

## Packaging

Build a local binary:

```sh
make build
```

Build a versioned release archive plus `checksums.txt`:

```sh
make package VERSION=v0.2.0
```

Build a cross-target release archive:

```sh
make release-archive VERSION=v0.2.0 GOOS=darwin GOARCH=arm64
```

Packaging notes:

- macOS and Linux archives are produced as `.tar.gz`
- Windows archives are produced as `.zip` and contain `transcriber_..._windows_amd64.exe`

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
