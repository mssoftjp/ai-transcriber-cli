# Changelog

## Unreleased

## v0.4.0

- Add an interactive Bubble Tea TUI for one-off local transcription jobs.
- Add API key management commands with macOS Keychain support and local key-file fallback.
- Use one API key resolution order across CLI, pipeline, doctor, and TUI flows.
- Refactor CLI spec building into a shared tested package used by CLI and TUI.
- Improve TUI option editing, selection controls, output-path hints, and API key configuration screens.
- Expand gitignore coverage for generated transcripts, local configs, scratch data, and private agent notes.

## v0.3.0

- Add `--parallel` client-chunk sending for `gpt-4o-transcribe` and `gpt-4o-mini-transcribe`.
- Keep sequential execution as the default and emit explicit warnings when parallel mode disables prompt carryover.
- Record client chunk execution mode in manifests and require `--resume` runs to match the original sequential vs. parallel mode.
- Treat `--parallel` as a no-op for single-request or server-chunked plans rather than failing the job.
- Add resumable client-chunked transcription via manifest-backed chunk caches.
- Default `--retries` to `1` for retryable provider failures and reset input streams correctly before retry.
- Refresh README and contract/config documentation for parallel chunking and resume behavior.

## v0.2.0

- Switch ffmpeg-generated intermediate audio from large WAV files to compact provider-ready `.m4a` artifacts.
- Keep workdir artifacts aligned with what was actually uploaded to the provider.
- Add a single-request runtime guard that rejects normalized audio exceeding the provider upload size limit.
- Update intermediate-size estimation to match compressed AAC/M4A output more closely.

## v0.1.1

- Fix GitHub Actions lint execution so CI works correctly with the repository Go version.
- Package Windows release artifacts as `.zip` archives with a `.exe` binary inside.
- Keep macOS and Linux release packaging unchanged as `.tar.gz`.

## v0.1.0

- Initial public release of the Go CLI.
- Add `transcribe`, `probe`, `doctor`, `version`, `config init`, and `config validate`.
- Support `txt`, `md`, `json`, `srt`, and `vtt` output formats.
- Add JSONL events, manifest output, dry-run planning, and partial-output handling.
- Add model-aware planning for `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`, `whisper-1`, `whisper-1-ts`, and diarize-capable transcription.
- Add ffmpeg/ffprobe-based probing, normalization, trimming, chunking, and local VAD boundary optimization.
- Add dictionary correction, AI postprocess, and speaker-aware rendering paths.
- Add release packaging, checksums, local hooks, CI workflows, and improved CLI help for automation wrappers.
