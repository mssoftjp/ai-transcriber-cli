package domain

import "time"

const (
	ProtocolVersion = "1.0"
	SchemaVersion   = "1"
)

type Stage string

const (
	StageCreated        Stage = "created"
	StageProbing        Stage = "probing"
	StageNormalizing    Stage = "normalizing"
	StagePlanning       Stage = "planning"
	StageTranscribing   Stage = "transcribing"
	StageMerging        Stage = "merging"
	StagePostprocessing Stage = "postprocessing"
	StageRendering      Stage = "rendering"
	StageWriting        Stage = "writing"
	StageCompleted      Stage = "completed"
	StagePartial        Stage = "partial"
	StageFailed         Stage = "failed"
	StageCancelled      Stage = "cancelled"
)

type ChunkingMode string

const (
	ChunkingAuto       ChunkingMode = "auto"
	ChunkingOff        ChunkingMode = "off"
	ChunkingServerAuto ChunkingMode = "server-auto"
	ChunkingServerVAD  ChunkingMode = "server-vad"
	ChunkingClient     ChunkingMode = "client"
)

type VADMode string

const (
	VADDisabled VADMode = "disabled"
	VADLocal    VADMode = "local"
)

type OutputFormat string

const (
	FormatTXT  OutputFormat = "txt"
	FormatMD   OutputFormat = "md"
	FormatJSON OutputFormat = "json"
	FormatSRT  OutputFormat = "srt"
	FormatVTT  OutputFormat = "vtt"
)

type EventsMode string

const (
	EventsText  EventsMode = "text"
	EventsJSONL EventsMode = "jsonl"
	EventsNone  EventsMode = "none"
)

type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

type PartialOutputMode string

const (
	PartialWrite   PartialOutputMode = "write"
	PartialDiscard PartialOutputMode = "discard"
	PartialStdout  PartialOutputMode = "stdout"
)

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MergeBoundaryDiagnostic struct {
	BoundaryIndex         int     `json:"boundary_index"`
	LeftChunkIndex        int     `json:"left_chunk_index"`
	RightChunkIndex       int     `json:"right_chunk_index"`
	Strategy              string  `json:"strategy"`
	OverlapSec            float64 `json:"overlap_sec"`
	LeftDurationSec       float64 `json:"left_duration_sec"`
	RightDurationSec      float64 `json:"right_duration_sec"`
	EstimatedOverlapRunes int     `json:"estimated_overlap_runes"`
	LeftTailRunes         int     `json:"left_tail_runes"`
	LocalWindowRunes      int     `json:"local_window_runes,omitempty"`
	SearchWindowStartRune int     `json:"search_window_start_rune"`
	SearchWindowEndRune   int     `json:"search_window_end_rune"`
	CandidateCount        int     `json:"candidate_count"`
	ChosenCutRune         int     `json:"chosen_cut_rune,omitempty"`
	OverlapScore          float64 `json:"overlap_score,omitempty"`
	NoveltyScore          float64 `json:"novelty_score,omitempty"`
	CombinedScore         float64 `json:"combined_score,omitempty"`
	ScoreGap              float64 `json:"score_gap,omitempty"`
	FallbackUsed          bool    `json:"fallback_used"`
}

type ChunkPlan struct {
	Index           int     `json:"index"`
	StartSec        float64 `json:"start_sec"`
	EndSec          float64 `json:"end_sec"`
	OverlapSec      float64 `json:"overlap_sec"`
	PromptCarryover bool    `json:"prompt_carryover"`
}

type Usage struct {
	Type         string  `json:"type"`
	Seconds      float64 `json:"seconds,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	TotalTokens  int64   `json:"total_tokens,omitempty"`
	AudioTokens  int64   `json:"audio_tokens,omitempty"`
	TextTokens   int64   `json:"text_tokens,omitempty"`
}

type Segment struct {
	ID               string   `json:"id"`
	StartSec         float64  `json:"start_sec"`
	EndSec           float64  `json:"end_sec"`
	Text             string   `json:"text"`
	Speaker          *string  `json:"speaker,omitempty"`
	Confidence       *float64 `json:"confidence,omitempty"`
	SourceChunkIndex int      `json:"source_chunk_index"`
}

type WordTiming struct {
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Word     string  `json:"word"`
}

type Transcript struct {
	Version     string       `json:"version"`
	Text        string       `json:"text"`
	Language    string       `json:"language"`
	ModelUsed   string       `json:"model_used"`
	Partial     bool         `json:"partial"`
	DurationSec float64      `json:"duration_sec"`
	Segments    []Segment    `json:"segments,omitempty"`
	Words       []WordTiming `json:"words,omitempty"`
	Speakers    []string     `json:"speakers,omitempty"`
	Usage       Usage        `json:"usage"`
	Warnings    []Warning    `json:"warnings,omitempty"`
}

type Artifact struct {
	Path         string       `json:"path"`
	Format       OutputFormat `json:"format"`
	Partial      bool         `json:"partial"`
	Bytes        int64        `json:"bytes,omitempty"`
	ManifestPath string       `json:"manifest_path,omitempty"`
}

type InputInfo struct {
	Path         string  `json:"path"`
	SizeBytes    int64   `json:"size_bytes,omitempty"`
	DurationSec  float64 `json:"duration_sec,omitempty"`
	Container    string  `json:"container,omitempty"`
	HasAudio     bool    `json:"has_audio"`
	IsVideo      bool    `json:"is_video,omitempty"`
	Extension    string  `json:"extension,omitempty"`
	FFmpegNeeded bool    `json:"ffmpeg_needed,omitempty"`
}

type Manifest struct {
	Version          string                    `json:"version"`
	JobID            string                    `json:"job_id"`
	Input            InputInfo                 `json:"input"`
	Plan             PlanSummary               `json:"plan"`
	Artifacts        []Artifact                `json:"artifacts,omitempty"`
	TimingsMS        Timings                   `json:"timings_ms"`
	Warnings         []Warning                 `json:"warnings,omitempty"`
	MergeDiagnostics []MergeBoundaryDiagnostic `json:"merge_diagnostics,omitempty"`
}

type PlanSummary struct {
	ChunkingMode ChunkingMode `json:"chunking_mode"`
	Model        string       `json:"model"`
	Language     string       `json:"language"`
}

type Timings struct {
	Probe       int64 `json:"probe,omitempty"`
	Normalize   int64 `json:"normalize,omitempty"`
	Transcribe  int64 `json:"transcribe,omitempty"`
	Merge       int64 `json:"merge,omitempty"`
	Postprocess int64 `json:"postprocess,omitempty"`
	Render      int64 `json:"render,omitempty"`
	Write       int64 `json:"write,omitempty"`
}

type ProbeResult struct {
	ProtocolVersion           string          `json:"protocol_version"`
	Input                     string          `json:"input"`
	Container                 string          `json:"container"`
	HasAudioStream            bool            `json:"has_audio_stream"`
	DurationSec               float64         `json:"duration_sec"`
	FFmpegRequired            bool            `json:"ffmpeg_required"`
	SingleRequestPossible     bool            `json:"single_request_possible"`
	PlannedChunkingMode       ChunkingMode    `json:"planned_chunking_mode"`
	OutputFormats             []FormatSupport `json:"output_formats"`
	DiarizeCapable            bool            `json:"diarize_capable"`
	TimestampCapable          bool            `json:"timestamp_capable"`
	EstimatedIntermediateSize int64           `json:"estimated_intermediate_size"`
}

type FormatSupport struct {
	Format    OutputFormat `json:"format"`
	Supported bool         `json:"supported"`
	Reason    string       `json:"reason,omitempty"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type DoctorResult struct {
	ProtocolVersion string        `json:"protocol_version"`
	GeneratedAt     time.Time     `json:"generated_at"`
	Checks          []DoctorCheck `json:"checks"`
	Warnings        []Warning     `json:"warnings,omitempty"`
}

type JobSpec struct {
	JobID                             string
	APIKeyEnv                         string
	InputPath                         string
	OutputPath                        string
	OutputDir                         string
	Format                            OutputFormat
	Model                             string
	Language                          string
	Prompt                            string
	Logprobs                          bool
	ChunkingMode                      ChunkingMode
	VADMode                           VADMode
	DictionaryPath                    string
	DictionaryEnabled                 bool
	Postprocess                       bool
	PostprocessModel                  string
	PostprocessPrompt                 string
	StartSec                          *float64
	EndSec                            *float64
	EventsMode                        EventsMode
	LogFormat                         LogFormat
	Quiet                             bool
	Verbose                           bool
	FFmpegPath                        string
	FFprobePath                       string
	KeepWorkdir                       bool
	Workdir                           string
	Timeout                           time.Duration
	Retries                           int
	PartialOutput                     PartialOutputMode
	Overwrite                         bool
	WriteManifest                     bool
	RawProviderJSONPath               string
	Stdout                            bool
	DryRun                            bool
	IncludeSegments                   bool
	AllowExperimentalDiarizeStitching bool
	ServerVADThreshold                float64
	ServerVADPrefixMS                 int
	ServerVADSilenceMS                int
	ChunkTargetSecOverride            *float64
	ChunkOverlapSecOverride           *float64
	SpeakerRefs                       []SpeakerReference
}

type SpeakerReference struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (u Usage) Add(other Usage) Usage {
	if u.Type == "" {
		u.Type = other.Type
	}
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.TotalTokens += other.TotalTokens
	u.AudioTokens += other.AudioTokens
	u.TextTokens += other.TextTokens
	u.Seconds += other.Seconds
	return u
}
