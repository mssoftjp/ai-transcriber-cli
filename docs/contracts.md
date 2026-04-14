# Contracts

Audience: GUI authors, automation wrappers, and other integrators who need stable machine-readable CLI behavior.

This document summarizes the main public contracts exposed by the CLI.

## CLI Commands

- `transcriber transcribe <input>`
- `transcriber probe <input>`
- `transcriber doctor`
- `transcriber version`
- `transcriber config init`
- `transcriber config validate`

## Core Output Types

### Transcript

- the canonical transcript includes `version`, `text`, `language`, `model_used`, `partial`, and `duration_sec`
- timestamp-capable models may include `segments` and `words`
- provider usage is stored under `usage`
- structured warnings are stored under `warnings`

### Manifest

- `job_id`
- `input`
- `plan`
- `artifacts`
- `timings_ms`
- `warnings`

## Minimal Examples

These examples are intentionally small. They show the payload shape that automation and wrappers should expect.

### `probe` output

```json
{
  "probe": {
    "protocol_version": "1.0",
    "input": "/abs/path/input.m4a",
    "container": "mov,mp4,m4a,3gp,3g2,mj2",
    "has_audio_stream": true,
    "duration_sec": 26.05,
    "ffmpeg_required": false,
    "single_request_possible": true,
    "planned_chunking_mode": "off",
    "output_formats": [
      { "format": "txt", "supported": true },
      { "format": "md", "supported": true },
      { "format": "json", "supported": true },
      { "format": "srt", "supported": false, "reason": "model does not provide timestamps" },
      { "format": "vtt", "supported": false, "reason": "model does not provide timestamps" }
    ],
    "diarize_capable": false,
    "timestamp_capable": false,
    "estimated_intermediate_size": 208400
  },
  "plan": {
    "input": {
      "path": "/abs/path/input.m4a",
      "size_bytes": 183244,
      "duration_sec": 26.05,
      "container": "mov,mp4,m4a,3gp,3g2,mj2",
      "has_audio": true,
      "extension": ".m4a"
    },
    "ffmpeg_required": false,
    "normalization_required": false,
    "single_request_possible": true,
    "chunking_mode": "off",
    "response_format": "json",
    "timestamp_capable": false,
    "diarize_capable": false,
    "estimated_intermediate_size": 208400,
    "artifacts": [
      {
        "path": "/abs/path/input.transcript.md",
        "format": "md",
        "partial": false,
        "manifest_path": "/abs/path/input.transcript.manifest.json"
      }
    ]
  }
}
```

### Transcript JSON output

```json
{
  "version": "1",
  "text": "This is a placeholder transcript used for documentation examples only.",
  "language": "english",
  "model_used": "gpt-4o-mini-transcribe",
  "partial": false,
  "duration_sec": 26.05,
  "usage": {
    "type": "tokens",
    "output_tokens": 69,
    "total_tokens": 69
  },
  "warnings": []
}
```

### Manifest JSON

```json
{
  "version": "1",
  "job_id": "job_01HXYZEXAMPLE",
  "input": {
    "path": "/abs/path/input.m4a",
    "size_bytes": 183244,
    "duration_sec": 26.05,
    "container": "mov,mp4,m4a,3gp,3g2,mj2",
    "has_audio": true,
    "extension": ".m4a"
  },
  "plan": {
    "chunking_mode": "off",
    "model": "gpt-4o-mini-transcribe",
    "language": "auto"
  },
  "artifacts": [
    {
      "path": "/abs/path/input.transcript.md",
      "format": "md",
      "partial": false,
      "bytes": 2148,
      "manifest_path": "/abs/path/input.transcript.manifest.json"
    }
  ],
  "timings_ms": {
    "probe": 37,
    "transcribe": 1642,
    "render": 2,
    "write": 1
  },
  "warnings": []
}
```

### Output Modes

- default mode writes transcript artifacts to disk
- `--stdout` writes only the transcript payload to stdout
- `--events jsonl` writes progress events to stdout
- `--stdout` and `--events jsonl` are mutually exclusive
- `--partial-output` controls failure behavior: `write`, `discard`, or `stdout`

## Event Contract

Top-level JSONL event fields:

- `type`
- `protocol_version`
- `job_id`
- `ts`
- `code` when applicable

Public events:

- `job.started`
- `job.planned`
- `stage.started`
- `stage.progress`
- `chunk.started`
- `chunk.completed`
- `warning`
- `artifact.written`
- `job.completed`
- `job.partial`
- `job.failed`
- `job.cancelled`

### Example JSONL events

```json
{"type":"job.started","protocol_version":"1.0","job_id":"job_01HXYZEXAMPLE","ts":"2026-04-14T09:00:00Z","input":"/abs/path/input.m4a"}
{"type":"job.planned","protocol_version":"1.0","job_id":"job_01HXYZEXAMPLE","ts":"2026-04-14T09:00:00Z","input":"/abs/path/input.m4a","plan":{"chunking_mode":"off","response_format":"json","timestamp_capable":false,"diarize_capable":false}}
{"type":"stage.started","protocol_version":"1.0","job_id":"job_01HXYZEXAMPLE","ts":"2026-04-14T09:00:00Z","stage":"transcribing","message":"transcribing audio"}
{"type":"artifact.written","protocol_version":"1.0","job_id":"job_01HXYZEXAMPLE","ts":"2026-04-14T09:00:02Z","artifact":{"path":"/abs/path/input.transcript.md","format":"md","partial":false,"bytes":2148,"manifest_path":"/abs/path/input.transcript.manifest.json"}}
{"type":"job.completed","protocol_version":"1.0","job_id":"job_01HXYZEXAMPLE","ts":"2026-04-14T09:00:02Z","artifacts":[{"path":"/abs/path/input.transcript.md","format":"md","partial":false,"bytes":2148,"manifest_path":"/abs/path/input.transcript.manifest.json"}],"duration_ms":1684}
```

Failure-oriented automation should also expect:

- `warning` with `code` and `message`
- `job.failed` with `code` and `message`
- `job.partial` when partial output is produced
- `job.cancelled` on cancellation

## Exit Codes

- `0`: success
- `2`: argument error
- `3`: config error
- `4`: missing dependency
- `5`: input error
- `6`: decode / ffmpeg error
- `7`: network error
- `8`: auth error
- `9`: rate limit
- `10`: provider error
- `11`: write error
- `12`: partial result written
- `130`: cancelled

## Compatibility Notes

- `srt` and `vtt` require a timestamp-capable model
- `gpt-4o-transcribe-diarize` does not support `prompt`
- long-form diarize chunk stitching requires `--allow-experimental-diarize-stitching`
- `--speaker-ref name=path` attempts to stabilize labels that match known speaker names across chunks
- `logprobs` is supported only by `gpt-4o-transcribe` and `gpt-4o-mini-transcribe`
- `probe` does not upload audio and is intended as the safe planning step for automation
- `transcribe --dry-run` returns planning data without calling the provider
