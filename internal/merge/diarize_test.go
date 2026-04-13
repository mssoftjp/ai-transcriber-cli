package merge

import (
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestPrepareChunkTranscriptLocalizesChunkSpeakersWithoutReferences(t *testing.T) {
	t.Parallel()

	speaker := "speaker_0"
	transcript := PrepareChunkTranscript(ChunkPreparationOptions{
		Model:                      "gpt-4o-transcribe-diarize",
		ChunkIndex:                 1,
		TotalChunks:                3,
		AllowExperimentalStitching: true,
	}, domain.Transcript{
		Speakers: []string{"speaker_0"},
		Segments: []domain.Segment{{
			Text:    "hello",
			Speaker: &speaker,
		}},
	})

	if len(transcript.Speakers) != 1 || transcript.Speakers[0] != "speaker_0@chunk-2" {
		t.Fatalf("unexpected localized speakers: %#v", transcript.Speakers)
	}
	if transcript.Segments[0].Speaker == nil || *transcript.Segments[0].Speaker != "speaker_0@chunk-2" {
		t.Fatalf("unexpected localized segment speaker: %#v", transcript.Segments[0])
	}
}

func TestPrepareChunkTranscriptKeepsKnownSpeakerReferencesStable(t *testing.T) {
	t.Parallel()

	speaker := "agent"
	transcript := PrepareChunkTranscript(ChunkPreparationOptions{
		Model:                      "gpt-4o-transcribe-diarize",
		ChunkIndex:                 0,
		TotalChunks:                2,
		AllowExperimentalStitching: true,
		KnownSpeakerNames:          []string{"agent", "customer"},
	}, domain.Transcript{
		Segments: []domain.Segment{{
			Text:    "hello",
			Speaker: &speaker,
		}},
	})

	if transcript.Segments[0].Speaker == nil || *transcript.Segments[0].Speaker != "agent" {
		t.Fatalf("expected known speaker label to stay stable, got %#v", transcript.Segments[0])
	}
	if len(transcript.Speakers) != 1 || transcript.Speakers[0] != "agent" {
		t.Fatalf("expected speakers to be collected from segments, got %#v", transcript.Speakers)
	}
}

func TestPrepareChunkTranscriptLocalizesUnknownSpeakerLabelsEvenWithReferences(t *testing.T) {
	t.Parallel()

	speaker := "speaker_0"
	transcript := PrepareChunkTranscript(ChunkPreparationOptions{
		Model:                      "gpt-4o-transcribe-diarize",
		ChunkIndex:                 2,
		TotalChunks:                4,
		AllowExperimentalStitching: true,
		KnownSpeakerNames:          []string{"Agent", "Customer"},
	}, domain.Transcript{
		Segments: []domain.Segment{{
			Text:    "hello",
			Speaker: &speaker,
		}},
	})

	if transcript.Segments[0].Speaker == nil || *transcript.Segments[0].Speaker != "speaker_0@chunk-3" {
		t.Fatalf("expected unknown speaker label to stay chunk-local, got %#v", transcript.Segments[0])
	}
	found := false
	for _, warning := range transcript.Warnings {
		if warning.Code == "diarize_unknown_speaker_labels" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown speaker warning, got %#v", transcript.Warnings)
	}
}

func TestPrepareChunkTranscriptCanonicalizesKnownSpeakerLabels(t *testing.T) {
	t.Parallel()

	speaker := " agent "
	transcript := PrepareChunkTranscript(ChunkPreparationOptions{
		Model:                      "gpt-4o-transcribe-diarize",
		ChunkIndex:                 0,
		TotalChunks:                2,
		AllowExperimentalStitching: true,
		KnownSpeakerNames:          []string{"Agent", "Customer"},
	}, domain.Transcript{
		Segments: []domain.Segment{{
			Text:    "hello",
			Speaker: &speaker,
		}},
	})

	if transcript.Segments[0].Speaker == nil || *transcript.Segments[0].Speaker != "Agent" {
		t.Fatalf("expected known speaker label to be canonicalized, got %#v", transcript.Segments[0])
	}
	if len(transcript.Speakers) != 1 || transcript.Speakers[0] != "Agent" {
		t.Fatalf("expected canonical speaker list, got %#v", transcript.Speakers)
	}
}
