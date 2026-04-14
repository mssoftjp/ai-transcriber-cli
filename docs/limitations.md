# Limitations

Audience: maintainers, reviewers, and adopters who want a concise view of known constraints and tradeoffs.

This document summarizes current implementation limits and the main areas still open for improvement.

## Current Constraints

- long-form quality depends more on `chunking` and `merge` than on the provider call itself
- `whisper-1` `verbose_json` usage follows the current SDK / response shape, which exposes duration-style usage rather than token-style usage
- `gpt-4o-transcribe-diarize` is more constrained than the standard transcription path, and long-form stitching is still a focused improvement area
- local VAD currently focuses on boundary optimization using `ffmpeg` silence detection rather than a dedicated VAD engine
- AI postprocess uses the Responses API at transcript scope and falls back safely to deterministic cleanup when the AI request fails

## Model-Specific Notes

### `whisper-1`

- provides timestamps
- uses shorter client chunk windows
- parallel chunk execution works well
- when a single language is forced strongly, code-switched audio may become easier to read at the cost of dropping other-language content

### `gpt-4o-transcribe` / `gpt-4o-mini-transcribe`

- optimized for transcript text quality
- use longer client chunk windows
- default to sequential execution plus prompt carryover
- optional `--parallel` works only for client-chunked runs and trades prompt carryover for speed
- may preserve mixed-language output more faithfully than a normalized monolingual transcript

### `gpt-4o-transcribe-diarize`

- does not support `prompt`
- expects short speaker reference clips
- treats long-form chunking more conservatively
- without `--speaker-ref`, long-form stitching keeps speaker labels chunk-local for safety
- even with `--speaker-ref`, labels that do not match known references remain chunk-local to avoid false merges

## Operational Caveats

- if `ffmpeg` or `ffprobe` is missing, some input types and chunking paths will not work
- with `--overwrite=false`, existing transcript, manifest, and raw provider JSON outputs are preserved
- resumable client-chunked runs must keep the same sequential vs. parallel execution mode
- GUIs and automation are expected to rely on the contracts in `docs/contracts.md` for events, manifests, and exit codes

## Planned Improvement Areas

- merge quality improvements
- more capable local VAD implementations
- stronger AI postprocess behavior
- better handling for long-form diarize stitching
- continued synchronization between docs and public CLI behavior
