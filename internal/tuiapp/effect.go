package tuiapp

import "ai-transcriber-cli/internal/domain"

type Effect interface{}

type RunProbe struct {
	RequestID string
}

type RunTranscription struct {
	RequestID string
}

type CancelRun struct {
	RequestID string
}

type ExitProgram struct{}

type SaveAPIKey struct {
	Method string
	Value  string
}

type DeleteAPIKey struct {
	Source string
}

type RunSpec struct {
	Spec domain.JobSpec
}
