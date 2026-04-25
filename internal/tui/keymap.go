package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ai-transcriber-cli/internal/tuiapp"
)

type keyAction struct {
	event       tuiapp.Event
	quit        bool
	editInput   bool
	submitInput bool
}

func translateKey(screen tuiapp.Screen, msg tea.KeyMsg) keyAction {
	switch screen {
	case tuiapp.ScreenSelfCheck:
		return translateSelfCheckKey(msg)
	case tuiapp.ScreenMenu:
		return translateMenuKey(msg)
	case tuiapp.ScreenOptions:
		return translateOptionsKey(msg)
	case tuiapp.ScreenAPIKey, tuiapp.ScreenSelect:
		return translateListKey(msg)
	case tuiapp.ScreenJobForm:
		return translateJobFormKey(msg)
	case tuiapp.ScreenEdit:
		return translateEditKey(msg)
	case tuiapp.ScreenConfirmLocalFile, tuiapp.ScreenConfirmDeleteAPIKey:
		return translateConfirmKey(msg)
	case tuiapp.ScreenConfirm:
		return translateRunConfirmKey(msg)
	case tuiapp.ScreenRunning:
		return translateRunningKey(msg)
	case tuiapp.ScreenExitConfirm:
		return translateExitConfirmKey(msg)
	case tuiapp.ScreenResult, tuiapp.ScreenError:
		return translateTerminalKey(msg)
	default:
		return keyAction{}
	}
}

func translateSelfCheckKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter":
		return keyAction{event: tuiapp.Submit{}}
	case "esc", "ctrl+c", "q":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateMenuKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "up", "k":
		return keyAction{event: tuiapp.MoveUp{}}
	case "down", "j":
		return keyAction{event: tuiapp.MoveDown{}}
	case "enter":
		return keyAction{event: tuiapp.Submit{}}
	case "esc", "ctrl+c", "q":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateListKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "up", "k":
		return keyAction{event: tuiapp.MoveUp{}}
	case "down", "j":
		return keyAction{event: tuiapp.MoveDown{}}
	case "enter":
		return keyAction{event: tuiapp.Submit{}}
	case "esc":
		return keyAction{event: tuiapp.Cancel{}}
	case "q":
		return keyAction{event: tuiapp.Cancel{}}
	case "ctrl+c":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateOptionsKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "up", "k":
		return keyAction{event: tuiapp.MoveUp{}}
	case "down", "j":
		return keyAction{event: tuiapp.MoveDown{}}
	case "left", "h":
		return keyAction{event: tuiapp.MoveLeft{}}
	case "right", "l":
		return keyAction{event: tuiapp.MoveRight{}}
	case "enter":
		return keyAction{event: tuiapp.Submit{}}
	case "esc", "q":
		return keyAction{event: tuiapp.Cancel{}}
	case "ctrl+c":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateJobFormKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "up", "k":
		return keyAction{event: tuiapp.MoveUp{}}
	case "down", "j":
		return keyAction{event: tuiapp.MoveDown{}}
	case "left", "h":
		return keyAction{event: tuiapp.MoveLeft{}}
	case "right", "l":
		return keyAction{event: tuiapp.MoveRight{}}
	case "enter":
		return keyAction{event: tuiapp.EditSelected{}}
	case "s":
		return keyAction{event: tuiapp.Submit{}}
	case "esc", "q":
		return keyAction{event: tuiapp.Cancel{}}
	case "ctrl+c":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateEditKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter":
		return keyAction{submitInput: true}
	case "esc":
		return keyAction{event: tuiapp.Cancel{}}
	case "ctrl+c":
		return keyAction{quit: true}
	default:
		return keyAction{editInput: true}
	}
}

func translateConfirmKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter", "y", "Y":
		return keyAction{event: tuiapp.ConfirmYes{}}
	case "esc", "n", "N":
		return keyAction{event: tuiapp.ConfirmNo{}}
	case "ctrl+c", "q":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateRunConfirmKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter", "s":
		return keyAction{event: tuiapp.Submit{}}
	case "esc":
		return keyAction{event: tuiapp.Cancel{}}
	case "ctrl+c", "q":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}

func translateRunningKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "esc", "ctrl+c":
		return keyAction{event: tuiapp.Cancel{}}
	default:
		return keyAction{}
	}
}

func translateExitConfirmKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter", "y", "Y":
		return keyAction{event: tuiapp.Submit{}}
	case "esc", "n", "N":
		return keyAction{event: tuiapp.Cancel{}}
	default:
		return keyAction{}
	}
}

func translateTerminalKey(msg tea.KeyMsg) keyAction {
	switch msg.String() {
	case "enter":
		return keyAction{event: tuiapp.Submit{}}
	case "q", "esc", "ctrl+c":
		return keyAction{quit: true}
	default:
		return keyAction{}
	}
}
