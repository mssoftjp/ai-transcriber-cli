package tuiapp

import (
	"errors"
	"testing"

	"ai-transcriber-cli/internal/domain"
)

func TestMenuSubmitStartsJobForm(t *testing.T) {
	state := InitialState()
	state.OptionForm.Model = "whisper-1"
	state.OptionForm.Format = "json"
	state.JobForm.Model = "gpt-4o-transcribe"

	next, effects := Update(state, Submit{})

	assertValid(t, next)
	if next.Screen != ScreenJobForm {
		t.Fatalf("Screen = %v, want JobForm", next.Screen)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
	if next.JobForm.Model != "whisper-1" || next.JobForm.Format != "json" || next.JobForm.Cursor != 0 {
		t.Fatalf("JobForm = %#v, want copied option defaults at start row", next.JobForm)
	}
}

func TestJobFormSubmitRunsProbe(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm

	next, effects := Update(state, Submit{})

	assertValid(t, next)
	if next.Screen != ScreenConfirm || next.Pending.Kind != RequestProbe {
		t.Fatalf("state = %#v, want confirm with pending probe", next)
	}
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one RunProbe", effects)
	}
	effect, ok := effects[0].(RunProbe)
	if !ok || effect.RequestID == "" || effect.RequestID != next.Pending.ID {
		t.Fatalf("effect = %#v, pending = %#v", effects[0], next.Pending)
	}
}

func TestJobFormEnterOnStartRunsProbe(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm
	state.JobForm.Cursor = 0

	next, effects := Update(state, EditSelected{})

	assertValid(t, next)
	if next.Screen != ScreenConfirm || next.Pending.Kind != RequestProbe {
		t.Fatalf("state = %#v, want confirm with pending probe", next)
	}
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one RunProbe", effects)
	}
}

func TestStaleProbeCompletionIsIgnored(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenConfirm
	state.Pending = RequestState{Kind: RequestProbe, ID: "current"}

	next, effects := Update(state, ProbeCompleted{RequestID: "old", Message: "old result"})

	assertValid(t, next)
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
	if next.Confirm.ProbeReady {
		t.Fatal("stale completion should not mark probe ready")
	}
	if next.Pending.ID != "current" {
		t.Fatalf("Pending = %#v, want unchanged", next.Pending)
	}
}

func TestStartRunRequiresProbeReady(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenConfirm

	next, effects := Update(state, StartRun{})

	assertValid(t, next)
	if next.Screen != ScreenConfirm {
		t.Fatalf("Screen = %v, want Confirm", next.Screen)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestStartRunIssuesTranscription(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenConfirm
	state.Confirm.ProbeReady = true

	next, effects := Update(state, StartRun{})

	assertValid(t, next)
	if next.Screen != ScreenRunning || next.Pending.Kind != RequestTranscription {
		t.Fatalf("state = %#v, want running transcription", next)
	}
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one RunTranscription", effects)
	}
	effect, ok := effects[0].(RunTranscription)
	if !ok || effect.RequestID != next.Pending.ID {
		t.Fatalf("effect = %#v, pending = %#v", effects[0], next.Pending)
	}
}

func TestRunningProgressIgnoresStaleRequest(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenRunning
	state.Pending = RequestState{Kind: RequestTranscription, ID: "current"}

	next, _ := Update(state, TranscriptionProgressed{
		RequestID: "old",
		Completed: 3,
		Total:     4,
		Stage:     domain.StageTranscribing,
		Message:   "old",
	})

	assertValid(t, next)
	if next.Running.Completed != 0 || next.Running.StatusMessage != "" {
		t.Fatalf("stale progress changed running state: %#v", next.Running)
	}
}

func TestCancelFlowRequiresConfirmation(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenRunning
	state.Pending = RequestState{Kind: RequestTranscription, ID: "run-1"}

	confirm, effects := Update(state, Cancel{})
	assertValid(t, confirm)
	if confirm.Screen != ScreenExitConfirm {
		t.Fatalf("Screen = %v, want ExitConfirm", confirm.Screen)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none before confirmation", effects)
	}

	cancelled, effects := Update(confirm, Submit{})
	assertValid(t, cancelled)
	if !cancelled.Running.CancelPending {
		t.Fatal("expected cancel pending")
	}
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one CancelRun", effects)
	}
	effect, ok := effects[0].(CancelRun)
	if !ok || effect.RequestID != "run-1" {
		t.Fatalf("effect = %#v, want CancelRun run-1", effects[0])
	}
}

func TestCompletionMovesToResultAndClearsPending(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenRunning
	state.Pending = RequestState{Kind: RequestTranscription, ID: "run-1"}

	next, effects := Update(state, TranscriptionCompleted{RequestID: "run-1"})

	assertValid(t, next)
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
	if next.Screen != ScreenResult || next.Pending.Kind != RequestNone {
		t.Fatalf("state = %#v, want result without pending", next)
	}
}

func TestFailureMovesToError(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenRunning
	state.Pending = RequestState{Kind: RequestTranscription, ID: "run-1"}

	next, _ := Update(state, TranscriptionFailed{RequestID: "run-1", Err: errors.New("boom")})

	assertValid(t, next)
	if next.Screen != ScreenError || next.Error.Message != "boom" {
		t.Fatalf("state = %#v, want error boom", next)
	}
}

func TestMoveAndInputUpdatesJobForm(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm

	state, _ = Update(state, MoveDown{})
	state, _ = Update(state, InputChanged{Field: "model", Value: "whisper-1"})
	state, _ = Update(state, InputChanged{Field: "prompt", Value: "names"})

	assertValid(t, state)
	if state.JobForm.Cursor != 1 {
		t.Fatalf("Cursor = %d, want 1", state.JobForm.Cursor)
	}
	if state.JobForm.Model != "whisper-1" || state.JobForm.Prompt != "names" {
		t.Fatalf("JobForm = %#v", state.JobForm)
	}
}

func TestMenuCanOpenOptionsAndQuit(t *testing.T) {
	state := InitialState()
	state.Menu.Cursor = 1

	options, effects := Update(state, Submit{})
	assertValid(t, options)
	if options.Screen != ScreenOptions || len(effects) != 0 {
		t.Fatalf("options state/effects = %#v/%#v", options, effects)
	}

	if len(InitialState().Menu.Items) != 2 {
		t.Fatalf("menu items = %#v, want Start/Option", InitialState().Menu.Items)
	}
}

func TestOptionsSubmitOpensAPIKeyScreen(t *testing.T) {
	state := InitialState()
	state.Menu.Cursor = 1

	state, _ = Update(state, Submit{})
	next, effects := Update(state, Submit{})

	assertValid(t, next)
	if next.Screen != ScreenAPIKey {
		t.Fatalf("Screen = %v, want APIKey", next.Screen)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestOptionsChangeInlineSelectors(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenOptions
	state.Options.Cursor = 2 // Language

	next, effects := Update(state, MoveRight{})
	assertValid(t, next)
	if next.OptionForm.Language != "ja" || next.JobForm.Language != "auto" {
		t.Fatalf("forms = option:%#v job:%#v, want option-only language change", next.OptionForm, next.JobForm)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	next.Options.Cursor = 3 // Output format
	next, _ = Update(next, MoveLeft{})
	if next.OptionForm.Format != "vtt" || next.JobForm.Format != "md" {
		t.Fatalf("forms = option:%#v job:%#v, want option-only format change", next.OptionForm, next.JobForm)
	}
}

func TestOptionsModelOpensSelectScreen(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenOptions
	state.Options.Cursor = 1 // Model

	selecting, effects := Update(state, Submit{})
	assertValid(t, selecting)
	if selecting.Screen != ScreenSelect || selecting.Select.Field != "model" {
		t.Fatalf("select state = %#v", selecting)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	selecting, _ = Update(selecting, MoveDown{})
	next, effects := Update(selecting, Submit{})
	assertValid(t, next)
	if next.Screen != ScreenOptions || next.OptionForm.Model != "gpt-4o-mini-transcribe" || next.JobForm.Model != "gpt-4o-transcribe" {
		t.Fatalf("state = %#v, want selected model", next)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestOptionsEnterCyclesInlineSelectors(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenOptions
	state.Options.Cursor = 2 // Language

	next, effects := Update(state, Submit{})
	assertValid(t, next)
	if next.OptionForm.Language != "ja" || next.JobForm.Language != "auto" {
		t.Fatalf("forms = option:%#v job:%#v, want option-only language change", next.OptionForm, next.JobForm)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestJobFormUsesSharedChoiceControls(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm
	state.JobForm.Cursor = 3 // Format
	state.OptionForm.Format = "json"

	next, effects := Update(state, MoveRight{})
	assertValid(t, next)
	if next.JobForm.Format != "txt" {
		t.Fatalf("Format = %q, want txt", next.JobForm.Format)
	}
	if next.OptionForm.Format != "json" {
		t.Fatalf("OptionForm.Format = %q, want unchanged json", next.OptionForm.Format)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	next.JobForm.Cursor = 4 // Language
	next, _ = Update(next, EditSelected{})
	if next.JobForm.Language != "ja" {
		t.Fatalf("Language = %q, want ja after enter", next.JobForm.Language)
	}
}

func TestJobFormModelOpensSelectScreen(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm
	state.JobForm.Cursor = 2 // Model

	selecting, effects := Update(state, EditSelected{})
	assertValid(t, selecting)
	if selecting.Screen != ScreenSelect || selecting.Select.Field != "model" || selecting.Select.ReturnScreen != ScreenJobForm {
		t.Fatalf("select state = %#v", selecting)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	selecting, _ = Update(selecting, MoveDown{})
	next, effects := Update(selecting, Submit{})
	assertValid(t, next)
	if next.Screen != ScreenJobForm || next.JobForm.Model != "gpt-4o-mini-transcribe" {
		t.Fatalf("state = %#v, want selected model on job form", next)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestEditSelectedUpdatesJobFormField(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenJobForm
	state.JobForm.Cursor = 6

	edit, effects := Update(state, EditSelected{})
	assertValid(t, edit)
	if edit.Screen != ScreenEdit || edit.Edit.Field != "prompt" {
		t.Fatalf("edit state = %#v", edit)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	next, effects := Update(edit, EditSubmitted{Value: "names"})
	assertValid(t, next)
	if next.Screen != ScreenJobForm || next.JobForm.Prompt != "names" {
		t.Fatalf("state = %#v, want edited job form", next)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}
}

func TestAPIKeyKeychainEditEmitsSave(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenAPIKey

	edit, effects := Update(state, Submit{})
	assertValid(t, edit)
	if edit.Screen != ScreenEdit || edit.APIKey.PendingMethod != "keychain" || !edit.Edit.Password {
		t.Fatalf("edit state = %#v", edit)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none", effects)
	}

	next, effects := Update(edit, EditSubmitted{Value: " sk-test "})
	assertValid(t, next)
	if next.Screen != ScreenAPIKey {
		t.Fatalf("Screen = %v, want APIKey", next.Screen)
	}
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one SaveAPIKey", effects)
	}
	effect, ok := effects[0].(SaveAPIKey)
	if !ok || effect.Method != "keychain" || effect.Value != "sk-test" {
		t.Fatalf("effect = %#v, want keychain save", effects[0])
	}
}

func TestAPIKeyFileSaveRequiresConfirmation(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenAPIKey
	state.APIKey.Cursor = 1

	edit, _ := Update(state, Submit{})
	confirm, effects := Update(edit, EditSubmitted{Value: "sk-file"})
	assertValid(t, confirm)
	if confirm.Screen != ScreenConfirmLocalFile {
		t.Fatalf("Screen = %v, want ConfirmLocalFile", confirm.Screen)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %#v, want none before confirmation", effects)
	}

	next, effects := Update(confirm, ConfirmYes{})
	assertValid(t, next)
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want one SaveAPIKey", effects)
	}
	effect, ok := effects[0].(SaveAPIKey)
	if !ok || effect.Method != "file" || effect.Value != "sk-file" {
		t.Fatalf("effect = %#v, want file save", effects[0])
	}
}

func TestAPIKeyDeleteOnlyDeletesStoredSources(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenAPIKey
	state.APIKey.Cursor = 3
	state.APIKey.ActiveSource = "openai_env"

	var effects []Effect
	next, _ := Update(state, Submit{})
	next, effects = Update(next, ConfirmYes{})
	assertValid(t, next)
	if len(effects) != 0 || next.APIKey.Status == "" {
		t.Fatalf("next/effects = %#v/%#v, want env delete explanation", next, effects)
	}

	state.APIKey.ActiveSource = "keychain"
	next, _ = Update(state, Submit{})
	next, effects = Update(next, ConfirmYes{})
	assertValid(t, next)
	if len(effects) != 1 {
		t.Fatalf("effects = %#v, want DeleteAPIKey", effects)
	}
	if effect, ok := effects[0].(DeleteAPIKey); !ok || effect.Source != "keychain" {
		t.Fatalf("effect = %#v, want keychain delete", effects[0])
	}
}

func TestCancelReturnsAuxiliaryScreensToMenu(t *testing.T) {
	for _, screen := range []Screen{ScreenOptions, ScreenAPIKey, ScreenProbe, ScreenEdit, ScreenConfirmLocalFile, ScreenConfirmDeleteAPIKey} {
		state := InitialState()
		state.Screen = screen
		if screen == ScreenEdit {
			state.APIKey.PendingMethod = "keychain"
		}

		next, effects := Update(state, Cancel{})

		assertValid(t, next)
		want := ScreenMenu
		if screen == ScreenAPIKey {
			want = ScreenOptions
		}
		if screen == ScreenEdit || screen == ScreenConfirmLocalFile || screen == ScreenConfirmDeleteAPIKey {
			want = ScreenAPIKey
		}
		if next.Screen != want {
			t.Fatalf("screen %v cancel -> %v, want %v", screen, next.Screen, want)
		}
		if len(effects) != 0 {
			t.Fatalf("effects = %#v, want none", effects)
		}
	}
}

func TestProgressAppendsSnippetWarningAndArtifact(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenRunning
	state.Pending = RequestState{Kind: RequestTranscription, ID: "run-1"}
	warning := domain.Warning{Code: "w", Message: "warning"}
	artifact := domain.Artifact{Path: "out.md", Format: domain.FormatMD}

	next, _ := Update(state, TranscriptionProgressed{
		RequestID: "run-1",
		Completed: 1,
		Total:     2,
		Stage:     domain.StageTranscribing,
		Message:   "working",
		Snippet:   "hello",
		Warning:   &warning,
		Artifact:  &artifact,
	})

	assertValid(t, next)
	if next.Running.Completed != 1 || next.Running.Total != 2 || next.Running.StatusMessage != "working" {
		t.Fatalf("Running = %#v", next.Running)
	}
	if len(next.Running.Snippets) != 1 || len(next.Running.Warnings) != 1 || len(next.Running.Artifacts) != 1 {
		t.Fatalf("Running lists = %#v", next.Running)
	}
}

func TestCancelledCompletionMovesToResult(t *testing.T) {
	state := InitialState()
	state.Screen = ScreenExitConfirm
	state.Pending = RequestState{Kind: RequestTranscription, ID: "run-1"}
	state.Running.CancelPending = true

	next, _ := Update(state, TranscriptionCancelled{RequestID: "run-1"})

	assertValid(t, next)
	if next.Screen != ScreenResult || next.Result.Status != domain.StageCancelled {
		t.Fatalf("state = %#v, want cancelled result", next)
	}
}

func FuzzUpdateDoesNotBreakState(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(2)
	f.Add(3)
	f.Fuzz(func(t *testing.T, choice int) {
		state := InitialState()
		events := []Event{
			MoveDown{},
			MoveUp{},
			Submit{},
			Cancel{},
			InputChanged{Field: "input", Value: "日本語.m4a"},
			ProbeCompleted{RequestID: state.Pending.ID, Message: "ok"},
		}
		for i := 0; i < 20; i++ {
			index := (choice + i) % len(events)
			if index < 0 {
				index = -index
			}
			event := events[index]
			state, _ = Update(state, event)
			if err := ValidateState(state); err != nil {
				t.Fatalf("ValidateState() after %#v: %v", event, err)
			}
		}
	})
}

func assertValid(t *testing.T, state State) {
	t.Helper()
	if err := ValidateState(state); err != nil {
		t.Fatalf("ValidateState() error = %v for %#v", err, state)
	}
}
