package tuiapp

import (
	"fmt"
	"strings"
)

func Update(s State, e Event) (State, []Effect) {
	switch event := e.(type) {
	case MoveUp:
		return moveCursor(s, -1), nil
	case MoveDown:
		return moveCursor(s, 1), nil
	case MoveLeft:
		return changeOption(s, -1), nil
	case MoveRight:
		return changeOption(s, 1), nil
	case InputChanged:
		return updateInput(s, event), nil
	case EditSelected:
		return editSelected(s)
	case EditSubmitted:
		return submitEdit(s, event.Value)
	case Submit:
		return submit(s)
	case StartRun:
		return startRun(s)
	case Cancel:
		return cancel(s)
	case ConfirmYes:
		return confirmYes(s)
	case ConfirmNo:
		return confirmNo(s), nil
	case APIKeyStatusChanged:
		return applyAPIKeyStatus(s, event), nil
	case APIKeySaved:
		return applyAPIKeySaved(s, event), nil
	case APIKeySaveFailed:
		return failAPIKeySave(s, event), nil
	case APIKeyDeleted:
		return applyAPIKeyDeleted(s, event), nil
	case APIKeyDeleteFailed:
		return failAPIKeyDelete(s, event), nil
	case ProbeCompleted:
		return probeCompleted(s, event), nil
	case ProbeFailed:
		return failRequest(s, RequestProbe, event.RequestID, event.Err), nil
	case TranscriptionProgressed:
		return transcriptionProgressed(s, event), nil
	case TranscriptionCompleted:
		return transcriptionCompleted(s, event), nil
	case TranscriptionFailed:
		return failRequest(s, RequestTranscription, event.RequestID, event.Err), nil
	case TranscriptionCancelled:
		return transcriptionCancelled(s, event), nil
	default:
		return s, nil
	}
}

func moveCursor(s State, delta int) State {
	switch s.Screen {
	case ScreenMenu:
		s.Menu.Cursor = clampCursor(s.Menu.Cursor+delta, len(s.Menu.Items))
	case ScreenOptions:
		s.Options.Cursor = clampCursor(s.Options.Cursor+delta, len(s.Options.Items))
	case ScreenJobForm:
		s.JobForm.Cursor = clampCursor(s.JobForm.Cursor+delta, 7)
	case ScreenAPIKey:
		s.APIKey.Cursor = clampCursor(s.APIKey.Cursor+delta, len(s.APIKey.Items))
	case ScreenSelect:
		s.Select.Cursor = clampCursor(s.Select.Cursor+delta, len(s.Select.Items))
	}
	return s
}

func updateInput(s State, event InputChanged) State {
	if s.Screen == ScreenEdit {
		s.Edit.Value = event.Value
		return s
	}
	if s.Screen != ScreenJobForm && s.Screen != ScreenOptions {
		return s
	}
	return setActiveFormField(s, event.Field, event.Value)
}

func setActiveFormField(s State, field, value string) State {
	return setFormFieldForScreen(s, s.Screen, field, value)
}

func setFormFieldForScreen(s State, screen Screen, field, value string) State {
	if screen == ScreenOptions {
		s.OptionForm = setFormField(s.OptionForm, field, value)
		return s
	}
	s.JobForm = setFormField(s.JobForm, field, value)
	return s
}

func setFormField(form JobFormState, field, value string) JobFormState {
	switch field {
	case "input":
		form.InputPath = value
	case "model":
		form.Model = value
	case "format":
		form.Format = value
	case "language":
		form.Language = value
	case "output":
		form.OutputPath = value
	case "prompt":
		form.Prompt = value
	}
	return form
}

func changeOption(s State, delta int) State {
	field, ok := selectedFormField(s)
	if !ok || len(field.Choices) == 0 {
		return s
	}
	form := formForScreen(s, s.Screen)
	s = setActiveFormField(s, field.Field, rotateValue(jobFieldValue(form, field.Field), field.Choices, delta))
	return s
}

func submit(s State) (State, []Effect) {
	switch s.Screen {
	case ScreenMenu:
		switch selectedMenuItem(s) {
		case "Start":
			s.JobForm = s.OptionForm
			s.JobForm.Cursor = 0
			s.Screen = ScreenJobForm
		case "Option":
			s.Screen = ScreenOptions
		}
	case ScreenOptions:
		switch selectedOptionItem(s) {
		case "API Key":
			s.Screen = ScreenAPIKey
			s.APIKey.Status = ""
		default:
			if field, ok := selectedFormField(s); ok {
				s = activateFormField(s, field, ScreenOptions)
			}
		}
	case ScreenJobForm:
		var requestID string
		s, requestID = issueRequest(s, RequestProbe)
		s.Screen = ScreenConfirm
		s.Confirm = ConfirmState{Message: "probing input"}
		return s, []Effect{RunProbe{RequestID: requestID}}
	case ScreenConfirm:
		if s.Confirm.ProbeReady {
			return startRun(s)
		}
	case ScreenExitConfirm:
		if s.Pending.Kind == RequestTranscription && s.Pending.ID != "" && !s.Running.CancelPending {
			s.Running.CancelPending = true
			return s, []Effect{CancelRun{RequestID: s.Pending.ID}}
		}
	case ScreenAPIKey:
		return submitAPIKey(s)
	case ScreenSelect:
		if s.Select.Cursor >= 0 && s.Select.Cursor < len(s.Select.Items) {
			value := s.Select.Items[s.Select.Cursor]
			s = setFormFieldForScreen(s, s.Select.ReturnScreen, s.Select.Field, value)
		}
		s.Screen = s.Select.ReturnScreen
		if s.Screen == 0 {
			s.Screen = ScreenOptions
		}
		s.Select = SelectState{}
	case ScreenError, ScreenResult:
		s.Screen = ScreenMenu
		s.Pending = RequestState{}
	}
	return s, nil
}

func startRun(s State) (State, []Effect) {
	if s.Screen != ScreenConfirm || !s.Confirm.ProbeReady || s.Pending.Kind != RequestNone {
		return s, nil
	}
	var requestID string
	s, requestID = issueRequest(s, RequestTranscription)
	s.Screen = ScreenRunning
	s.Running = RunningState{StatusMessage: "starting transcription"}
	return s, []Effect{RunTranscription{RequestID: requestID}}
}

func editSelected(s State) (State, []Effect) {
	if s.Screen == ScreenJobForm && s.JobForm.Cursor == 0 {
		return submit(s)
	}
	field, ok := selectedFormField(s)
	if !ok {
		return s, nil
	}
	return activateFormField(s, field, ScreenJobForm), nil
}

func openSelect(s State, title, field string, items []string, returnScreen Screen) State {
	if returnScreen == 0 {
		returnScreen = ScreenOptions
	}
	s.Select = SelectState{
		Title:        title,
		Items:        append([]string(nil), items...),
		Cursor:       indexOf(items, jobFieldValue(formForScreen(s, returnScreen), field)),
		Field:        field,
		ReturnScreen: returnScreen,
	}
	s.Screen = ScreenSelect
	return s
}

func activateFormField(s State, field formField, returnScreen Screen) State {
	if len(field.Choices) > 0 {
		if field.SelectTitle != "" {
			return openSelect(s, field.SelectTitle, field.Field, field.Choices, returnScreen)
		}
		form := formForScreen(s, returnScreen)
		s = setFormFieldForScreen(s, returnScreen, field.Field, rotateValue(jobFieldValue(form, field.Field), field.Choices, 1))
		return s
	}
	s.Edit = EditState{
		Title:        "Edit " + field.Title,
		Value:        jobFieldValue(formForScreen(s, returnScreen), field.Field),
		Field:        field.Field,
		ReturnScreen: returnScreen,
	}
	s.Screen = ScreenEdit
	return s
}

func cancel(s State) (State, []Effect) {
	switch s.Screen {
	case ScreenRunning:
		s.Screen = ScreenExitConfirm
	case ScreenExitConfirm:
		s.Screen = ScreenRunning
	case ScreenJobForm:
		s.Screen = ScreenMenu
	case ScreenConfirm:
		s.Screen = ScreenJobForm
		s.Pending = RequestState{}
	case ScreenOptions, ScreenProbe:
		s.Screen = ScreenMenu
	case ScreenAPIKey:
		s.Screen = ScreenOptions
	case ScreenSelect:
		s.Screen = s.Select.ReturnScreen
		if s.Screen == 0 {
			s.Screen = ScreenOptions
		}
		s.Select = SelectState{}
	case ScreenEdit:
		if s.Edit.Field != "" {
			s.Screen = s.Edit.ReturnScreen
			if s.Screen == 0 {
				s.Screen = ScreenJobForm
			}
			s.Edit = EditState{}
			return s, nil
		}
		s.Screen = ScreenAPIKey
		s.Edit = EditState{}
		s.APIKey.PendingMethod = ""
		s.APIKey.PendingValue = ""
	case ScreenConfirmLocalFile, ScreenConfirmDeleteAPIKey:
		s.Screen = ScreenAPIKey
		s.APIKey.Status = "Cancelled"
	case ScreenError, ScreenResult:
		s.Screen = ScreenMenu
		s.Pending = RequestState{}
	}
	return s, nil
}

func submitAPIKey(s State) (State, []Effect) {
	switch s.APIKey.Cursor {
	case 0:
		s.APIKey.PendingMethod = apiKeyMethodKeychain
		s.Edit = EditState{Title: "Enter API key", Password: true}
		s.Screen = ScreenEdit
	case 1:
		s.APIKey.PendingMethod = apiKeyMethodFile
		s.Edit = EditState{Title: "Enter API key", Password: true}
		s.Screen = ScreenEdit
	case 2:
		s.APIKey.Status = fmt.Sprintf("Set %s in your shell, then restart or continue with Keychain.", valueOr(s.APIKey.EnvName, "OPENAI_API_KEY"))
	case 3:
		s.Screen = ScreenConfirmDeleteAPIKey
		s.APIKey.Status = ""
	}
	return s, nil
}

func submitEdit(s State, value string) (State, []Effect) {
	if s.Screen != ScreenEdit {
		return s, nil
	}
	value = strings.TrimSpace(value)
	if s.Edit.Field != "" {
		field := s.Edit.Field
		returnScreen := s.Edit.ReturnScreen
		s = setFormFieldForScreen(s, returnScreen, field, value)
		s.Screen = returnScreen
		if s.Screen == 0 {
			s.Screen = ScreenJobForm
		}
		s.Edit = EditState{}
		return s, nil
	}
	if value == "" {
		s.APIKey.Status = "API key must not be empty"
		return s, nil
	}
	s.APIKey.PendingValue = value
	if s.APIKey.PendingMethod == apiKeyMethodFile {
		s.Screen = ScreenConfirmLocalFile
		return s, nil
	}
	method := s.APIKey.PendingMethod
	s.Screen = ScreenAPIKey
	s.APIKey.PendingMethod = ""
	s.APIKey.PendingValue = ""
	s.Edit = EditState{}
	return s, []Effect{SaveAPIKey{Method: method, Value: value}}
}

func confirmYes(s State) (State, []Effect) {
	switch s.Screen {
	case ScreenConfirmLocalFile:
		value := strings.TrimSpace(s.APIKey.PendingValue)
		if value == "" {
			s.APIKey.Status = "API key must not be empty"
			return s, nil
		}
		method := s.APIKey.PendingMethod
		s.Screen = ScreenAPIKey
		s.APIKey.PendingMethod = ""
		s.APIKey.PendingValue = ""
		s.Edit = EditState{}
		return s, []Effect{SaveAPIKey{Method: method, Value: value}}
	case ScreenConfirmDeleteAPIKey:
		source := s.APIKey.ActiveSource
		if source == "" || source == apiKeySourceNone || source == apiKeySourceOpenAIEnv || source == apiKeySourceConfiguredEnv {
			s.Screen = ScreenAPIKey
			s.APIKey.Status = "No stored Keychain or file key to delete"
			return s, nil
		}
		s.Screen = ScreenAPIKey
		return s, []Effect{DeleteAPIKey{Source: source}}
	default:
		return s, nil
	}
}

func jobFieldValue(form JobFormState, field string) string {
	switch field {
	case "input":
		return form.InputPath
	case "model":
		return form.Model
	case "format":
		return form.Format
	case "language":
		return form.Language
	case "output":
		return form.OutputPath
	case "prompt":
		return form.Prompt
	default:
		return ""
	}
}

func formForScreen(s State, screen Screen) JobFormState {
	if screen == ScreenOptions {
		return s.OptionForm
	}
	return s.JobForm
}

func confirmNo(s State) State {
	switch s.Screen {
	case ScreenConfirmLocalFile, ScreenConfirmDeleteAPIKey:
		s.Screen = ScreenAPIKey
		s.APIKey.Status = "Cancelled"
	}
	return s
}

func applyAPIKeyStatus(s State, event APIKeyStatusChanged) State {
	s.APIKey.CurrentStatus = valueOr(event.CurrentStatus, s.APIKey.CurrentStatus)
	s.APIKey.Method = valueOr(event.Method, s.APIKey.Method)
	s.APIKey.EnvName = valueOr(event.EnvName, s.APIKey.EnvName)
	s.APIKey.ActiveSource = event.ActiveSource
	s.APIKey.FilePath = event.FilePath
	s.APIKey.FileWarning = event.FileWarning
	if event.Status != "" {
		s.APIKey.Status = event.Status
	}
	return s
}

func applyAPIKeySaved(s State, event APIKeySaved) State {
	s.APIKey.Status = valueOr(event.Status, "API key saved")
	return s
}

func failAPIKeySave(s State, event APIKeySaveFailed) State {
	s.APIKey.Status = "Failed to save API key: " + errorMessage(event.Err)
	return s
}

func applyAPIKeyDeleted(s State, event APIKeyDeleted) State {
	s.APIKey.Status = valueOr(event.Status, "Saved key deleted")
	return s
}

func failAPIKeyDelete(s State, event APIKeyDeleteFailed) State {
	s.APIKey.Status = "Failed to delete API key: " + errorMessage(event.Err)
	return s
}

func probeCompleted(s State, event ProbeCompleted) State {
	if s.Screen != ScreenConfirm || !matchesPending(s, RequestProbe, event.RequestID) {
		return s
	}
	s.Confirm = ConfirmState{ProbeReady: true, Message: event.Message}
	s.Pending = RequestState{}
	return s
}

func transcriptionProgressed(s State, event TranscriptionProgressed) State {
	if s.Screen != ScreenRunning || !matchesPending(s, RequestTranscription, event.RequestID) {
		return s
	}
	s.Running.Completed = event.Completed
	s.Running.Total = event.Total
	s.Running.CurrentStage = event.Stage
	s.Running.StatusMessage = event.Message
	if event.Snippet != "" {
		s.Running.Snippets = append(s.Running.Snippets, event.Snippet)
	}
	if event.Warning != nil {
		s.Running.Warnings = append(s.Running.Warnings, *event.Warning)
	}
	if event.Artifact != nil {
		s.Running.Artifacts = append(s.Running.Artifacts, *event.Artifact)
	}
	return s
}

func transcriptionCompleted(s State, event TranscriptionCompleted) State {
	if !matchesPending(s, RequestTranscription, event.RequestID) {
		return s
	}
	s.Screen = ScreenResult
	s.Result = ResultState{Status: "completed", Artifacts: event.Artifacts}
	s.Pending = RequestState{}
	s.Running.CancelPending = false
	return s
}

func transcriptionCancelled(s State, event TranscriptionCancelled) State {
	if !matchesPending(s, RequestTranscription, event.RequestID) {
		return s
	}
	s.Screen = ScreenResult
	s.Result = ResultState{Status: "cancelled", Message: "cancelled"}
	s.Pending = RequestState{}
	s.Running.CancelPending = false
	return s
}

func failRequest(s State, kind RequestKind, requestID string, err error) State {
	if !matchesPending(s, kind, requestID) {
		return s
	}
	s.Screen = ScreenError
	s.Error = ErrorState{Message: errorMessage(err)}
	s.Pending = RequestState{}
	s.Running.CancelPending = false
	return s
}

func selectedMenuItem(s State) string {
	if len(s.Menu.Items) == 0 || s.Menu.Cursor < 0 || s.Menu.Cursor >= len(s.Menu.Items) {
		return ""
	}
	return s.Menu.Items[s.Menu.Cursor]
}

func selectedOptionItem(s State) string {
	if s.Options.Cursor < 0 || s.Options.Cursor >= len(s.Options.Items) {
		return ""
	}
	return s.Options.Items[s.Options.Cursor]
}

type formField struct {
	Field       string
	Title       string
	Choices     []string
	SelectTitle string
}

func selectedFormField(s State) (formField, bool) {
	switch s.Screen {
	case ScreenOptions:
		return optionFormField(selectedOptionItem(s))
	case ScreenJobForm:
		return jobFormField(s.JobForm.Cursor)
	default:
		return formField{}, false
	}
}

func optionFormField(item string) (formField, bool) {
	switch item {
	case "Model":
		return formField{Field: "model", Title: "Model", Choices: modelChoices(), SelectTitle: "Select model"}, true
	case "Language":
		return formField{Field: "language", Title: "Language", Choices: languageChoices()}, true
	case "Output format":
		return formField{Field: "format", Title: "Output format", Choices: formatChoices()}, true
	case "Output path":
		return formField{Field: "output", Title: "Output path"}, true
	case "Prompt":
		return formField{Field: "prompt", Title: "Prompt"}, true
	default:
		return formField{}, false
	}
}

func jobFormField(cursor int) (formField, bool) {
	switch cursor {
	case 1:
		return formField{Field: "input", Title: "Input file"}, true
	case 2:
		return formField{Field: "model", Title: "Model", Choices: modelChoices(), SelectTitle: "Select model"}, true
	case 3:
		return formField{Field: "format", Title: "Format", Choices: formatChoices()}, true
	case 4:
		return formField{Field: "language", Title: "Language", Choices: languageChoices()}, true
	case 5:
		return formField{Field: "output", Title: "Output path"}, true
	case 6:
		return formField{Field: "prompt", Title: "Prompt"}, true
	default:
		return formField{}, false
	}
}

func modelChoices() []string {
	return []string{"gpt-4o-transcribe", "gpt-4o-mini-transcribe", "whisper-1"}
}

func languageChoices() []string {
	return []string{"auto", "ja", "en"}
}

func formatChoices() []string {
	return []string{"md", "txt", "json", "srt", "vtt"}
}

func rotateValue(current string, values []string, delta int) string {
	if len(values) == 0 {
		return current
	}
	index := indexOf(values, current)
	if index < 0 {
		index = 0
	}
	index = (index + delta) % len(values)
	if index < 0 {
		index += len(values)
	}
	return values[index]
}

func indexOf(values []string, current string) int {
	for i, value := range values {
		if value == current {
			return i
		}
	}
	return 0
}

func matchesPending(s State, kind RequestKind, id string) bool {
	return s.Pending.Kind == kind && s.Pending.ID != "" && s.Pending.ID == id
}

func clampCursor(cursor, length int) int {
	if length <= 0 {
		return 0
	}
	if cursor < 0 {
		return length - 1
	}
	if cursor >= length {
		return 0
	}
	return cursor
}

func issueRequest(s State, kind RequestKind) (State, string) {
	s.nextRequestSeq++
	id := fmt.Sprintf("%d-%d", kind, s.nextRequestSeq)
	s.Pending = RequestState{Kind: kind, ID: id}
	return s, id
}

func errorMessage(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

const (
	apiKeyMethodKeychain      = "keychain"
	apiKeyMethodFile          = "file"
	apiKeySourceNone          = "none"
	apiKeySourceOpenAIEnv     = "openai_env"
	apiKeySourceConfiguredEnv = "configured_env"
)
