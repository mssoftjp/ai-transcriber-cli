package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ai-transcriber-cli/internal/tuiapp"
)

func TestTranslateKeyScreenActions(t *testing.T) {
	tests := []struct {
		name        string
		screen      tuiapp.Screen
		key         string
		eventType   any
		quit        bool
		editInput   bool
		submitInput bool
	}{
		{name: "menu down", screen: tuiapp.ScreenMenu, key: "j", eventType: tuiapp.MoveDown{}},
		{name: "menu quit", screen: tuiapp.ScreenMenu, key: "esc", quit: true},
		{name: "list back", screen: tuiapp.ScreenAPIKey, key: "esc", eventType: tuiapp.Cancel{}},
		{name: "options right", screen: tuiapp.ScreenOptions, key: "right", eventType: tuiapp.MoveRight{}},
		{name: "options back", screen: tuiapp.ScreenOptions, key: "q", eventType: tuiapp.Cancel{}},
		{name: "job right", screen: tuiapp.ScreenJobForm, key: "right", eventType: tuiapp.MoveRight{}},
		{name: "job edit", screen: tuiapp.ScreenJobForm, key: "enter", eventType: tuiapp.EditSelected{}},
		{name: "job back", screen: tuiapp.ScreenJobForm, key: "q", eventType: tuiapp.Cancel{}},
		{name: "job start", screen: tuiapp.ScreenJobForm, key: "s", eventType: tuiapp.Submit{}},
		{name: "edit submit", screen: tuiapp.ScreenEdit, key: "enter", submitInput: true},
		{name: "edit input", screen: tuiapp.ScreenEdit, key: "a", editInput: true},
		{name: "confirm yes", screen: tuiapp.ScreenConfirmLocalFile, key: "y", eventType: tuiapp.ConfirmYes{}},
		{name: "running cancel", screen: tuiapp.ScreenRunning, key: "ctrl+c", eventType: tuiapp.Cancel{}},
		{name: "exit no", screen: tuiapp.ScreenExitConfirm, key: "n", eventType: tuiapp.Cancel{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateKey(tt.screen, keyMsg(tt.key))
			if got.quit != tt.quit || got.editInput != tt.editInput || got.submitInput != tt.submitInput {
				t.Fatalf("flags = quit:%v edit:%v submit:%v", got.quit, got.editInput, got.submitInput)
			}
			if tt.eventType == nil {
				if got.event != nil {
					t.Fatalf("event = %#v, want nil", got.event)
				}
				return
			}
			if got.event == nil {
				t.Fatalf("event = nil, want %T", tt.eventType)
			}
			if gotType, wantType := eventName(got.event), eventName(tt.eventType); gotType != wantType {
				t.Fatalf("event = %s, want %s", gotType, wantType)
			}
		})
	}
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func eventName(event any) string {
	switch event.(type) {
	case tuiapp.MoveDown:
		return "MoveDown"
	case tuiapp.MoveRight:
		return "MoveRight"
	case tuiapp.MoveLeft:
		return "MoveLeft"
	case tuiapp.Cancel:
		return "Cancel"
	case tuiapp.EditSelected:
		return "EditSelected"
	case tuiapp.Submit:
		return "Submit"
	case tuiapp.ConfirmYes:
		return "ConfirmYes"
	default:
		return "unknown"
	}
}
