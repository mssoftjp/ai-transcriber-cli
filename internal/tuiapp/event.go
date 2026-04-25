package tuiapp

import "ai-transcriber-cli/internal/domain"

type Event interface{}

type MoveUp struct{}
type MoveDown struct{}
type MoveLeft struct{}
type MoveRight struct{}
type Submit struct{}
type Cancel struct{}
type StartRun struct{}
type EditSelected struct{}

type EditSubmitted struct {
	Value string
}

type ConfirmYes struct{}
type ConfirmNo struct{}

type InputChanged struct {
	Field string
	Value string
}

type APIKeyStatusChanged struct {
	CurrentStatus string
	Method        string
	EnvName       string
	ActiveSource  string
	FilePath      string
	FileWarning   string
	Status        string
}

type APIKeySaved struct {
	Method string
	Status string
}

type APIKeySaveFailed struct {
	Err error
}

type APIKeyDeleted struct {
	Source string
	Status string
}

type APIKeyDeleteFailed struct {
	Err error
}

type ProbeCompleted struct {
	RequestID string
	Message   string
}

type ProbeFailed struct {
	RequestID string
	Err       error
}

type TranscriptionProgressed struct {
	RequestID string
	Completed int
	Total     int
	Stage     domain.Stage
	Message   string
	Snippet   string
	Warning   *domain.Warning
	Artifact  *domain.Artifact
}

type TranscriptionCompleted struct {
	RequestID string
	Artifacts []domain.Artifact
}

type TranscriptionFailed struct {
	RequestID string
	Err       error
}

type TranscriptionCancelled struct {
	RequestID string
}
