package tuiview

import (
	"fmt"
	"path/filepath"
	"strings"

	"ai-transcriber-cli/internal/tuiapp"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81"))
	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246"))
	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	selectedRowStyle = lipgloss.NewStyle().
				Bold(true)
	activeSegmentStyle = lipgloss.NewStyle().
				Reverse(true)
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)
)

type Viewport struct {
	Width  int
	Height int
}

func Render(state tuiapp.State, vp Viewport) string {
	width := vp.Width
	if width <= 0 {
		width = 80
	}
	var lines []string
	switch state.Screen {
	case tuiapp.ScreenSelfCheck:
		lines = append(lines, titleStyle.Render("Self check"))
	case tuiapp.ScreenMenu:
		lines = append(lines, renderMenu(state, width)...)
	case tuiapp.ScreenJobForm:
		lines = append(lines, renderJobForm(state, width)...)
	case tuiapp.ScreenConfirm:
		lines = append(lines, renderConfirm(state)...)
	case tuiapp.ScreenRunning, tuiapp.ScreenExitConfirm:
		lines = append(lines, renderRunning(state)...)
		if state.Screen == tuiapp.ScreenExitConfirm {
			lines = append(lines, "", warningStyle.Render("Cancel this run?")+" "+footerStyle.Render("[Enter]/[y] Yes  [Esc]/[n] No"))
		}
	case tuiapp.ScreenResult:
		lines = append(lines, renderResult(state)...)
	case tuiapp.ScreenOptions:
		lines = append(lines, renderOptions(state, width)...)
	case tuiapp.ScreenAPIKey:
		lines = append(lines, renderAPIKey(state, width)...)
	case tuiapp.ScreenEdit:
		lines = append(lines, renderEdit(state)...)
	case tuiapp.ScreenSelect:
		lines = append(lines, renderSelect(state, width)...)
	case tuiapp.ScreenConfirmLocalFile:
		lines = append(lines,
			titleStyle.Render("Confirm Local File Save"),
			"",
			"This stores the API key in a local file protected by file permissions.",
			"For better security, use Keychain.",
			"",
			footerStyle.Render("[Enter]/[y] Save  [Esc]/[n] Cancel"),
		)
	case tuiapp.ScreenConfirmDeleteAPIKey:
		lines = append(lines, renderDeleteConfirm(state)...)
	case tuiapp.ScreenProbe:
		lines = append(lines, titleStyle.Render("Probe"), "Use the Transcribe path to probe an input before running.")
	case tuiapp.ScreenError:
		lines = append(lines, errorStyle.Render("Error"))
		for _, line := range Wrap(state.Error.Message, width) {
			lines = append(lines, errorStyle.Render(line))
		}
		lines = append(lines, "", footerStyle.Render("[Enter] Menu  [q] Quit"))
	default:
		lines = append(lines, errorStyle.Render("Unknown screen"))
	}
	for i := range lines {
		lines[i] = fitRendered(lines[i], width)
	}
	if vp.Height > 0 && len(lines) > vp.Height {
		lines = lines[:vp.Height]
	}
	return strings.Join(lines, "\n")
}

func renderAPIKey(state tuiapp.State, width int) []string {
	api := state.APIKey
	lines := []string{titleStyle.Render("API Key Configuration"), "", subtitleStyle.Render(fmt.Sprintf("Method: %s", apiKeyMethodDisplay(api)))}
	lines = append(lines, "")
	for i, item := range api.Items {
		selected := i == api.Cursor
		label := item
		if item == "Local file" && api.FilePath != "" {
			label = "Local file (" + api.FilePath + ")"
		}
		line := fmt.Sprintf(" %s %s ", cursor(selected), label)
		if selected {
			line = selectedRow(line, width)
		} else {
			line = menuItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", statusLine("Status", displayValue(api.CurrentStatus, "Not checked")))
	if api.EnvName != "" {
		lines = append(lines, subtitleStyle.Render(fmt.Sprintf("Variable: %s", api.EnvName)))
	}
	if api.ActiveSource != "" {
		lines = append(lines, subtitleStyle.Render(fmt.Sprintf("Active: %s", api.ActiveSource)))
	}
	if api.FileWarning != "" {
		lines = append(lines, warningStyle.Render(api.FileWarning))
	}
	lines = append(lines, "", subtitleStyle.Render("Secret value is never displayed."), footerStyle.Render("[Enter] Select  [Esc] Back"))
	if api.Status != "" {
		lines = append(lines, "", statusStyle.Render(api.Status))
	}
	return lines
}

func renderEdit(state tuiapp.State) []string {
	title := titleStyle.Render(displayValue(state.Edit.Title, "Edit"))
	input := "<hidden>"
	if !state.Edit.Password {
		input = displayValue(state.Edit.Value, "-")
	}
	return []string{title, "", statusStyle.Render(input), "", footerStyle.Render("[Enter] Save  [Esc] Cancel")}
}

func renderDeleteConfirm(state tuiapp.State) []string {
	source := displayValue(state.APIKey.ActiveSource, "none")
	lines := []string{titleStyle.Render("Confirm Key Deletion"), "", statusLine("Current source", source)}
	if source == "openai_env" || source == "configured_env" {
		lines = append(lines, warningStyle.Render("Environment variables are not removed from your shell by the TUI."))
	} else {
		lines = append(lines, "Only the currently active stored key is deleted.")
	}
	return append(lines, "", footerStyle.Render("[Enter]/[y] Delete  [Esc]/[n] Cancel"))
}

func renderMenu(state tuiapp.State, width int) []string {
	lines := []string{titleStyle.Render("ai-transcriber"), ""}
	for i, item := range state.Menu.Items {
		selected := i == state.Menu.Cursor
		line := fmt.Sprintf(" %s %s ", cursor(selected), item)
		if selected {
			line = selectedRow(line, width)
		} else {
			line = menuItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return append(lines, "", footerStyle.Render("[Enter] Select  [Esc] Quit"))
}

func renderOptions(state tuiapp.State, width int) []string {
	labelWidth := 20
	if width > 0 {
		labelWidth = max(8, min(20, width/2))
	}
	lines := []string{titleStyle.Render("Option"), ""}
	for i, item := range state.Options.Items {
		selected := i == state.Options.Cursor
		label := PadRight(Truncate(item, labelWidth), labelWidth)
		valueWidth := max(1, width-2-labelWidth-1)
		value := fitRendered(optionValue(state, item, selected), valueWidth)
		marker := optionMarker(item)
		if marker != "" {
			if selected {
				marker = selectedText(marker)
			}
			value = fitRendered(strings.TrimSpace(value+" "+marker), valueWidth)
		}
		line := selectableRow(selected, cursor(selected), label, value)
		if selected {
			line = fitRendered(line, width)
		} else {
			line = menuItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", footerStyle.Render("↑/↓ move   ←/→ change   enter edit/select   q back"))
	if state.Options.Status != "" {
		lines = append(lines, "", statusStyle.Render(state.Options.Status))
	}
	return lines
}

func optionValue(state tuiapp.State, item string, selected bool) string {
	switch item {
	case "API Key":
		return selectableValue(displayValue(state.APIKey.CurrentStatus, "Not checked"), selected)
	case "Model":
		return selectableValue(displayValue(state.OptionForm.Model, "-"), selected)
	case "Language":
		return segmentedValue(state.OptionForm.Language, []string{"auto", "ja", "en"}, selected)
	case "Output format":
		return segmentedValue(state.OptionForm.Format, []string{"md", "txt", "json", "srt", "vtt"}, selected)
	case "Output path":
		return selectableValue(outputPathDisplay(state.OptionForm.OutputPath, "", state.OptionForm.Format), selected)
	case "Prompt":
		return selectableValue(promptDisplay(state.OptionForm.Prompt), selected)
	default:
		return ""
	}
}

func optionMarker(item string) string {
	switch item {
	case "API Key", "Model":
		return ">"
	default:
		return ""
	}
}

func segmentedValue(current string, values []string, selected bool) string {
	if strings.TrimSpace(current) == "" {
		current = values[0]
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value == current {
			parts = append(parts, activeSegmentStyle.Render(" "+value+" "))
			continue
		}
		parts = append(parts, selectableValue(" "+value+" ", selected))
	}
	separator := "|"
	if selected {
		separator = selectedText(separator)
	}
	return selectableValue("[", selected) + strings.Join(parts, separator) + selectableValue("]", selected)
}

func renderSelect(state tuiapp.State, width int) []string {
	title := displayValue(state.Select.Title, "Select")
	lines := []string{titleStyle.Render(title), ""}
	for i, item := range state.Select.Items {
		selected := i == state.Select.Cursor
		line := fmt.Sprintf(" %s %s", cursor(selected), item)
		if selected {
			line = selectedRow(line, width)
		} else {
			line = menuItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return append(lines, "", footerStyle.Render("enter select   esc cancel"))
}

func displayValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func renderJobForm(state tuiapp.State, width int) []string {
	items := []string{"Start", "Input file", "Model", "Output format", "Language", "Output path", "Prompt"}
	labelWidth := 20
	if width > 0 {
		labelWidth = max(8, min(20, width/2))
	}
	lines := []string{titleStyle.Render("Job"), ""}
	for i, item := range items {
		selected := i == state.JobForm.Cursor
		label := PadRight(Truncate(item, labelWidth), labelWidth)
		valueWidth := max(1, width-2-labelWidth-1)
		value := fitRendered(jobFormValue(state, item, selected), valueWidth)
		if marker := jobFormMarker(item); marker != "" {
			if selected {
				marker = selectedText(marker)
			}
			value = fitRendered(strings.TrimSpace(value+" "+marker), valueWidth)
		}
		line := selectableRow(selected, cursor(selected), label, value)
		if selected {
			line = fitRendered(line, width)
		} else {
			line = menuItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return append(lines, "", footerStyle.Render("↑/↓ move   ←/→ change   enter edit/select   s start   q back"))
}

func jobFormValue(state tuiapp.State, item string, selected bool) string {
	switch item {
	case "Start":
		return selectableValue("Probe and transcribe", selected)
	case "Input file":
		return selectableValue(inputPathDisplay(state.JobForm.InputPath), selected)
	case "Model":
		return selectableValue(displayValue(state.JobForm.Model, "-"), selected)
	case "Output format":
		return segmentedValue(state.JobForm.Format, []string{"md", "txt", "json", "srt", "vtt"}, selected)
	case "Language":
		return segmentedValue(state.JobForm.Language, []string{"auto", "ja", "en"}, selected)
	case "Output path":
		return selectableValue(outputPathDisplay(state.JobForm.OutputPath, state.JobForm.InputPath, state.JobForm.Format), selected)
	case "Prompt":
		return selectableValue(promptDisplay(state.JobForm.Prompt), selected)
	default:
		return ""
	}
}

func jobFormMarker(item string) string {
	switch item {
	case "Model":
		return ">"
	default:
		return ""
	}
}

func inputPathDisplay(value string) string {
	return displayValue(value, "not set")
}

func outputPathDisplay(value, inputPath, format string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	if strings.TrimSpace(format) == "" {
		format = "md"
	}
	if strings.TrimSpace(inputPath) == "" {
		return "same folder as input"
	}
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "same folder as input"
	}
	return "same folder: " + base + ".transcript." + format
}

func promptDisplay(value string) string {
	return displayValue(value, "none")
}

func renderConfirm(state tuiapp.State) []string {
	status := state.Confirm.Message
	if status == "" {
		status = "waiting for probe"
	}
	lines := []string{titleStyle.Render("Confirm"), statusStyle.Render(status)}
	if state.Pending.Kind == tuiapp.RequestProbe {
		lines = append(lines, statusStyle.Render("Probe is running..."))
	}
	if state.Confirm.ProbeReady {
		lines = append(lines, footerStyle.Render("[Enter] Start  [Esc] Back"))
	}
	return lines
}

func renderRunning(state tuiapp.State) []string {
	total := state.Running.Total
	if total == 0 {
		total = 1
	}
	lines := []string{
		titleStyle.Render("Running"),
		subtitleStyle.Render(fmt.Sprintf("Stage: %s", state.Running.CurrentStage)),
		statusStyle.Render(fmt.Sprintf("Progress: %d/%d", state.Running.Completed, total)),
		statusLine("Status", state.Running.StatusMessage),
	}
	if len(state.Running.Warnings) > 0 {
		last := state.Running.Warnings[len(state.Running.Warnings)-1]
		lines = append(lines, warningStyle.Render(fmt.Sprintf("Warning: %s", last.Message)))
	}
	if len(state.Running.Artifacts) > 0 {
		last := state.Running.Artifacts[len(state.Running.Artifacts)-1]
		lines = append(lines, successStyle.Render(fmt.Sprintf("Artifact: %s", last.Path)))
	}
	if len(state.Running.Snippets) > 0 {
		lines = append(lines, subtitleStyle.Render("Recent:"), state.Running.Snippets[len(state.Running.Snippets)-1])
	}
	lines = append(lines, "", footerStyle.Render("[Esc/Ctrl-C] Cancel"))
	return lines
}

func renderResult(state tuiapp.State) []string {
	lines := []string{titleStyle.Render("Result"), statusLine("Status", string(state.Result.Status))}
	if state.Result.Message != "" {
		lines = append(lines, state.Result.Message)
	}
	for _, artifact := range state.Result.Artifacts {
		lines = append(lines, successStyle.Render(fmt.Sprintf("%s: %s", artifact.Format, artifact.Path)))
	}
	return append(lines, "", footerStyle.Render("[Enter] Menu  [q] Quit"))
}

func apiKeyMethodDisplay(api tuiapp.APIKeyState) string {
	switch api.ActiveSource {
	case "openai_env":
		return "Environment variable"
	case "configured_env":
		return "Environment variable"
	case "keychain":
		return "Keychain"
	case "file":
		return "Local file"
	}
	switch api.Method {
	case "keychain":
		return "Keychain"
	case "file":
		return "Local file"
	case "env":
		return "Environment variable"
	default:
		return displayValue(api.Method, "auto")
	}
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return Truncate(value, width)
}

func fitRendered(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if DisplayWidth(value) <= width {
		return value
	}
	return fit(stripANSI(value), width)
}

func selectedRow(value string, width int) string {
	value = fitRendered(value, width)
	return selectedRowStyle.Render(strings.TrimRight(value, " "))
}

func selectableRow(selected bool, marker, label, value string) string {
	if selected {
		marker = selectedText(marker)
		label = selectedText(label)
	}
	return fmt.Sprintf(" %s %s %s ", marker, label, value)
}

func selectableValue(value string, selected bool) string {
	if selected {
		return selectedText(value)
	}
	return value
}

func selectedText(value string) string {
	return selectedRowStyle.Render(value)
}

func statusLine(label, value string) string {
	return subtitleStyle.Render(label+": ") + statusStyle.Render(displayValue(value, "-"))
}

func cursor(selected bool) string {
	if selected {
		return "›"
	}
	return " "
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
