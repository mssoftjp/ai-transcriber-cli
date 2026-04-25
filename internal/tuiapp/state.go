package tuiapp

import (
	"time"

	"ai-transcriber-cli/internal/domain"
)

type Screen int

const (
	ScreenSelfCheck Screen = iota
	ScreenMenu
	ScreenJobForm
	ScreenConfirm
	ScreenRunning
	ScreenResult
	ScreenOptions
	ScreenAPIKey
	ScreenEdit
	ScreenSelect
	ScreenConfirmLocalFile
	ScreenConfirmDeleteAPIKey
	ScreenProbe
	ScreenError
	ScreenExitConfirm
)

type State struct {
	Screen     Screen
	Menu       MenuState
	Options    OptionsState
	Select     SelectState
	APIKey     APIKeyState
	Edit       EditState
	OptionForm JobFormState
	JobForm    JobFormState
	Confirm    ConfirmState
	Running    RunningState
	Result     ResultState
	Error      ErrorState
	Pending    RequestState

	nextRequestSeq int
}

type MenuState struct {
	Items  []string
	Cursor int
}

type OptionsState struct {
	Items  []string
	Cursor int
	Status string
}

type SelectState struct {
	Title        string
	Items        []string
	Cursor       int
	Field        string
	ReturnScreen Screen
}

type APIKeyState struct {
	Items         []string
	Cursor        int
	CurrentStatus string
	Method        string
	EnvName       string
	ActiveSource  string
	FilePath      string
	FileWarning   string
	Status        string
	PendingMethod string
	PendingValue  string
}

type EditState struct {
	Title        string
	Value        string
	Password     bool
	Field        string
	ReturnScreen Screen
}

type JobFormState struct {
	InputPath  string
	Model      string
	Format     string
	Language   string
	OutputPath string
	Prompt     string
	Cursor     int
}

type ConfirmState struct {
	ProbeReady bool
	Message    string
}

type RunningState struct {
	StartedAt     time.Time
	Completed     int
	Total         int
	CurrentStage  domain.Stage
	StatusMessage string
	Snippets      []string
	Warnings      []domain.Warning
	Artifacts     []domain.Artifact
	CancelPending bool
}

type ResultState struct {
	Status    domain.Stage
	Artifacts []domain.Artifact
	Message   string
}

type ErrorState struct {
	Message string
}

type RequestKind int

const (
	RequestNone RequestKind = iota
	RequestSelfCheck
	RequestProbe
	RequestTranscription
	RequestAPIKey
	RequestSaveConfig
)

type RequestState struct {
	Kind RequestKind
	ID   string
}

func InitialState() State {
	defaultForm := JobFormState{
		Model:    "gpt-4o-transcribe",
		Format:   "md",
		Language: "auto",
	}
	return State{
		Screen: ScreenMenu,
		Menu: MenuState{
			Items:  []string{"Start", "Option"},
			Cursor: 0,
		},
		Options: OptionsState{
			Items: []string{
				"API Key",
				"Model",
				"Language",
				"Output format",
				"Output path",
				"Prompt",
			},
		},
		APIKey: APIKeyState{
			Items: []string{
				"Keychain (recommended)",
				"Local file",
				"Environment variable",
				"Delete current key",
			},
			CurrentStatus: "Not checked",
			Method:        "auto",
			EnvName:       "OPENAI_API_KEY",
		},
		OptionForm: defaultForm,
		JobForm:    defaultForm,
	}
}
