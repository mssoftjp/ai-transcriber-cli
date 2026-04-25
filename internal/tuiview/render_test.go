package tuiview

import (
	"strings"
	"testing"

	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/tuiapp"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderMenuNarrowViewport(t *testing.T) {
	out := Render(tuiapp.InitialState(), Viewport{Width: 10, Height: 5})
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 10 {
			t.Fatalf("line %q width = %d, want <= 10", line, DisplayWidth(line))
		}
	}
}

func TestRenderJobFormJapaneseTruncatesSafely(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenJobForm
	state.JobForm.InputPath = "日本語のとても長いファイル名.m4a"

	out := Render(state, Viewport{Width: 18, Height: 20})
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 18 {
			t.Fatalf("line %q width = %d, want <= 18", line, DisplayWidth(line))
		}
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("render did not truncate: %q", out)
	}
}

func TestRenderRunningIncludesWarningAndArtifact(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenRunning
	state.Running.CurrentStage = domain.StageTranscribing
	state.Running.Completed = 1
	state.Running.Total = 2
	state.Running.Warnings = []domain.Warning{{Code: "w", Message: "careful"}}
	state.Running.Artifacts = []domain.Artifact{{Path: "out.md", Format: domain.FormatMD}}

	out := Render(state, Viewport{Width: 80, Height: 24})
	if !strings.Contains(out, "careful") || !strings.Contains(out, "out.md") {
		t.Fatalf("render = %q, want warning and artifact", out)
	}
}

func TestRenderOptionsHighlightsCurrentSegment(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenOptions
	state.Options.Cursor = 2
	state.OptionForm.Language = "ja"

	out := Render(state, Viewport{Width: 80, Height: 24})
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("rendered = %q, want ANSI styling", out)
	}
	if strings.Contains(out, "38;5;62") || strings.Contains(out, "48;5;230") {
		t.Fatalf("rendered = %q, active segment should use reverse only without custom colors", out)
	}
	plain := stripANSI(out)
	if strings.Contains(plain, "Input file") {
		t.Fatalf("rendered = %q, want options without input file", plain)
	}
	if !strings.Contains(plain, "[ auto | ja | en ]") {
		t.Fatalf("rendered = %q, want segmented options", plain)
	}
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 80 {
			t.Fatalf("line %q width = %d, want <= 80", line, DisplayWidth(line))
		}
	}
}

func TestSegmentedValueUsesReverseOnlyForActiveChoice(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)

	out := segmentedValue("ja", []string{"auto", "ja", "en"}, false)
	if !strings.Contains(out, "\x1b[7m ja ") {
		t.Fatalf("segmentedValue = %q, want reverse active choice", out)
	}
	if strings.Contains(out, "38;5;") || strings.Contains(out, "48;5;") || strings.Contains(out, "\x1b[1;7m") {
		t.Fatalf("segmentedValue = %q, want reverse only without color or bold on active choice", out)
	}
}

func TestSelectedSegmentedRowKeepsBoldAfterActiveChoice(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)

	out := segmentedValue("ja", []string{"auto", "ja", "en"}, true)
	if !strings.Contains(out, activeSegmentStyle.Render(" ja ")) {
		t.Fatalf("segmentedValue = %q, want reverse active choice", out)
	}
	if !strings.Contains(out, selectedText(" en ")) {
		t.Fatalf("segmentedValue = %q, want trailing inactive choice explicitly bold", out)
	}
}

func TestRenderJobFormUsesChoiceControls(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenJobForm
	state.JobForm.Cursor = 3
	state.JobForm.Format = "txt"
	state.JobForm.Language = "ja"

	out := Render(state, Viewport{Width: 80, Height: 24})
	plain := stripANSI(out)
	for _, want := range []string{"Start", "Probe and transcribe", "Output format", "[ md | txt | json | srt | vtt ]", "[ auto | ja | en ]", "enter edit/select"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q, want %q", plain, want)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 80 {
			t.Fatalf("line %q width = %d, want <= 80", line, DisplayWidth(line))
		}
	}
}

func TestRenderJobFormShowsFieldStateWithoutEditMarkers(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenJobForm
	state.JobForm.InputPath = ""
	state.JobForm.OutputPath = ""
	state.JobForm.Prompt = ""

	out := Render(state, Viewport{Width: 80, Height: 24})
	plain := stripANSI(out)
	for _, want := range []string{"Input file", "not set", "Output path", "same folder as input", "Prompt", "none"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q, want %q", plain, want)
		}
	}
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "Input file") && strings.Contains(line, ">") {
			t.Fatalf("input file should not show detail marker: %q", line)
		}
		if strings.Contains(line, "Output path") && strings.Contains(line, ">") {
			t.Fatalf("output path should not show detail marker: %q", line)
		}
		if strings.Contains(line, "Prompt") && strings.Contains(line, ">") {
			t.Fatalf("prompt should not show detail marker: %q", line)
		}
	}
}

func TestRenderJobFormShowsConcreteDefaultOutputFromInput(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenJobForm
	state.JobForm.InputPath = "/Users/me/audio/meeting.m4a"
	state.JobForm.OutputPath = ""
	state.JobForm.Format = "txt"

	out := Render(state, Viewport{Width: 80, Height: 24})
	plain := stripANSI(out)
	if !strings.Contains(plain, "same folder: meeting.transcript.txt") {
		t.Fatalf("rendered = %q, want concrete default output path", plain)
	}
}

func TestRenderOptionsShowsOptionalTextFieldStateWithoutMarkers(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenOptions
	state.OptionForm.OutputPath = ""
	state.OptionForm.Prompt = ""

	out := Render(state, Viewport{Width: 80, Height: 24})
	plain := stripANSI(out)
	for _, want := range []string{"Output path", "same folder as input", "Prompt", "none"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q, want %q", plain, want)
		}
	}
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "Output path") && strings.Contains(line, ">") {
			t.Fatalf("output path should not show detail marker: %q", line)
		}
		if strings.Contains(line, "Prompt") && strings.Contains(line, ">") {
			t.Fatalf("prompt should not show detail marker: %q", line)
		}
	}
}

func TestWidthHelpers(t *testing.T) {
	if got := DisplayWidth("abc日本"); got != 7 {
		t.Fatalf("DisplayWidth = %d, want 7", got)
	}
	if got := DisplayWidth("\x1b[31mabc日本\x1b[0m"); got != 7 {
		t.Fatalf("DisplayWidth with ANSI = %d, want 7", got)
	}
	if got := PadRight("日本", 6); DisplayWidth(got) != 6 {
		t.Fatalf("PadRight width = %d, want 6", DisplayWidth(got))
	}
}

func TestRenderErrorWrapsLongMessage(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenError
	state.Error.Message = "OPENAI_API_KEY is required. Export it first, then re-run the command."

	out := Render(state, Viewport{Width: 24, Height: 20})
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 24 {
			t.Fatalf("line %q width = %d, want <= 24", line, DisplayWidth(line))
		}
	}
	if !strings.Contains(out, "Export it") || !strings.Contains(out, "first") {
		t.Fatalf("rendered = %q, want wrapped error details", out)
	}
}

func TestRenderAPIKeyConfiguration(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenAPIKey
	state.APIKey.CurrentStatus = "Registered (keychain)"
	state.APIKey.ActiveSource = "keychain"
	state.APIKey.FilePath = "/tmp/key.txt"

	out := Render(state, Viewport{Width: 44, Height: 24})
	for _, line := range strings.Split(out, "\n") {
		if DisplayWidth(line) > 44 {
			t.Fatalf("line %q width = %d, want <= 44", line, DisplayWidth(line))
		}
	}
	for _, want := range []string{"API Key Configuration", "Keychain (recommended)", "Registered (keychain)", "Secret value is never displayed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered = %q, want %q", out, want)
		}
	}
}

func TestRenderAPIKeyEditHidesSecret(t *testing.T) {
	state := tuiapp.InitialState()
	state.Screen = tuiapp.ScreenEdit
	state.APIKey.PendingMethod = "keychain"
	state.Edit.Title = "Enter API key"
	state.Edit.Value = "sk-secret"
	state.Edit.Password = true

	out := Render(state, Viewport{Width: 80, Height: 24})
	if strings.Contains(out, "sk-secret") || !strings.Contains(out, "<hidden>") {
		t.Fatalf("rendered = %q, want hidden secret", out)
	}
}
