package merge

import (
	"fmt"
	"strings"

	"ai-transcriber-cli/internal/domain"
)

type ChunkPreparationOptions struct {
	Model                      string
	ChunkIndex                 int
	TotalChunks                int
	AllowExperimentalStitching bool
	KnownSpeakerNames          []string
}

func PrepareChunkTranscript(options ChunkPreparationOptions, transcript domain.Transcript) domain.Transcript {
	if options.Model != "gpt-4o-transcribe-diarize" || options.TotalChunks <= 1 {
		return transcript
	}
	if !options.AllowExperimentalStitching {
		return transcript
	}
	transcript.Warnings = domain.AppendWarning(transcript.Warnings, domain.Warning{
		Code:    "diarize_stitching_experimental",
		Message: "long-form diarize stitching is experimental",
	})
	// Some provider responses only surface speakers on segments, so normalize the
	// top-level list here before later merge/render stages consume it.
	transcript.Speakers = collectSpeakers(transcript)
	if len(options.KnownSpeakerNames) > 0 {
		transcript, localizedUnknown := stabilizeKnownSpeakers(options, transcript)
		if localizedUnknown {
			transcript.Warnings = domain.AppendWarning(transcript.Warnings, domain.Warning{
				Code:    "diarize_unknown_speaker_labels",
				Message: "speaker labels outside the configured references were kept chunk-local to avoid cross-chunk collisions",
			})
		}
		return transcript
	}
	transcript.Warnings = domain.AppendWarning(transcript.Warnings, domain.Warning{
		Code:    "diarize_chunk_local_speakers",
		Message: "speaker labels are treated as chunk-local because no known speaker references were provided",
	})
	// Without stable speaker references, the safest option is to make chunk-local
	// labels explicit so later merge steps do not accidentally collapse speakers.
	rename := func(label string) string {
		if strings.TrimSpace(label) == "" {
			return label
		}
		return fmt.Sprintf("%s@chunk-%d", label, options.ChunkIndex+1)
	}
	transcript.Speakers = renameSpeakerList(transcript.Speakers, rename)
	for i := range transcript.Segments {
		if transcript.Segments[i].Speaker == nil {
			continue
		}
		renamed := rename(*transcript.Segments[i].Speaker)
		transcript.Segments[i].Speaker = &renamed
	}
	return transcript
}

func stabilizeKnownSpeakers(options ChunkPreparationOptions, transcript domain.Transcript) (domain.Transcript, bool) {
	known := canonicalSpeakerNames(options.KnownSpeakerNames)
	if len(known) == 0 {
		return transcript, false
	}
	localizedUnknown := false
	// Only labels that match a configured reference are made stable across chunks.
	// Unknown labels stay chunk-local so we do not accidentally merge different
	// speakers under the same global name during long-form stitching.
	rename := func(label string) string {
		normalized := normalizeSpeakerLabel(label)
		if normalized == "" {
			return label
		}
		if canonical, ok := known[normalized]; ok {
			return canonical
		}
		localizedUnknown = true
		return fmt.Sprintf("%s@chunk-%d", strings.TrimSpace(label), options.ChunkIndex+1)
	}
	transcript.Speakers = renameSpeakerList(transcript.Speakers, rename)
	for i := range transcript.Segments {
		if transcript.Segments[i].Speaker == nil {
			continue
		}
		renamed := rename(*transcript.Segments[i].Speaker)
		transcript.Segments[i].Speaker = &renamed
	}
	return transcript, localizedUnknown
}

func collectSpeakers(transcript domain.Transcript) []string {
	out := make([]string, 0, len(transcript.Segments)+len(transcript.Speakers))
	for _, speaker := range transcript.Speakers {
		if strings.TrimSpace(speaker) != "" {
			out = append(out, speaker)
		}
	}
	for _, segment := range transcript.Segments {
		if segment.Speaker != nil && strings.TrimSpace(*segment.Speaker) != "" {
			out = append(out, *segment.Speaker)
		}
	}
	return dedupeSpeakerList(out)
}

func renameSpeakerList(values []string, rename func(string) string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, rename(value))
	}
	return dedupeSpeakerList(out)
}

func dedupeSpeakerList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func canonicalSpeakerNames(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		normalized := normalizeSpeakerLabel(value)
		if normalized == "" {
			continue
		}
		if _, exists := out[normalized]; exists {
			continue
		}
		out[normalized] = strings.TrimSpace(value)
	}
	return out
}

func normalizeSpeakerLabel(value string) string {
	fields := strings.Fields(strings.TrimSpace(strings.ToLower(value)))
	return strings.Join(fields, " ")
}
