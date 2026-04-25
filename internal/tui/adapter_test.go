package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/keystore"
	"ai-transcriber-cli/internal/plan"
	"ai-transcriber-cli/internal/tuiapp"
)

type fakeRunner struct {
	probeErr      error
	transcribeErr error
}

type fakeEnv struct {
	values map[string]string
}

func (f fakeEnv) Get(key string) string {
	return f.values[key]
}

type fakeSecretStore struct {
	value   string
	set     string
	deleted bool
	err     error
}

func (f *fakeSecretStore) Get(string, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.value == "" {
		return "", keystore.ErrNotFound
	}
	return f.value, nil
}

func (f *fakeSecretStore) Set(_, _, value string) error {
	if f.err != nil {
		return f.err
	}
	f.set = value
	f.value = value
	return nil
}

func (f *fakeSecretStore) Delete(string, string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = true
	f.value = ""
	return nil
}

type fakeFileStore struct {
	path    string
	value   string
	written string
	deleted bool
}

func (f *fakeFileStore) Path(config.AppConfig) string {
	if f.path == "" {
		return "/tmp/key.txt"
	}
	return f.path
}

func (f *fakeFileStore) Read(config.AppConfig) (string, error) {
	if f.value == "" {
		return "", keystore.ErrNotFound
	}
	return f.value, nil
}

func (f *fakeFileStore) Write(_ config.AppConfig, value string) error {
	f.written = value
	f.value = value
	return nil
}

func (f *fakeFileStore) Delete(config.AppConfig) error {
	f.deleted = true
	f.value = ""
	return nil
}

func (f fakeRunner) Probe(context.Context, domain.JobSpec) (domain.ProbeResult, plan.ExecutionPlan, error) {
	if f.probeErr != nil {
		return domain.ProbeResult{}, plan.ExecutionPlan{}, f.probeErr
	}
	return domain.ProbeResult{DurationSec: 12}, plan.ExecutionPlan{ChunkingMode: domain.ChunkingOff}, nil
}

func (f fakeRunner) Transcribe(context.Context, domain.JobSpec) ([]domain.Artifact, []byte, error) {
	if f.transcribeErr != nil {
		return nil, nil, f.transcribeErr
	}
	return []domain.Artifact{{Path: "out.md", Format: domain.FormatMD}}, nil, nil
}

func testKeyResolver() keystore.Resolver {
	return keystore.Resolver{
		Env:      fakeEnv{values: map[string]string{}},
		Keychain: &fakeSecretStore{},
		Files:    &fakeFileStore{},
	}
}

func TestRunScriptShortestPath(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(form tuiapp.JobFormState) (domain.JobSpec, error) {
			if form.InputPath != "input.m4a" {
				t.Fatalf("InputPath = %q, want input.m4a", form.InputPath)
			}
			return domain.JobSpec{JobID: "job", InputPath: form.InputPath, Model: form.Model, Format: domain.FormatMD}, nil
		},
		Runner: fakeRunner{},
	}, Options{
		Out:         &out,
		AllowNonTTY: true,
		Script: []tuiapp.Event{
			tuiapp.Submit{},
			tuiapp.InputChanged{Field: "input", Value: "input.m4a"},
			tuiapp.Submit{},
			tuiapp.Submit{},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "Result") || !strings.Contains(rendered, "out.md") {
		t.Fatalf("rendered = %q, want result artifact", rendered)
	}
}

func TestRunRejectsNonTTYByDefault(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{}, nil },
		Runner:    fakeRunner{},
	}, Options{In: strings.NewReader(""), Out: &out})
	if err == nil {
		t.Fatal("expected non-TTY error")
	}
	if code := domain.ErrorCode(err); code != "tui_not_supported" {
		t.Fatalf("error code = %q, want tui_not_supported", code)
	}
}

func TestRunScriptShowsProbeError(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{probeErr: errors.New("probe failed")},
	}, Options{
		Out:         &out,
		AllowNonTTY: true,
		Script:      []tuiapp.Event{tuiapp.Submit{}, tuiapp.Submit{}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "probe failed") {
		t.Fatalf("rendered = %q, want probe failed", out.String())
	}
}

func TestRunInteractiveLines(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(form tuiapp.JobFormState) (domain.JobSpec, error) {
			return domain.JobSpec{JobID: "job", InputPath: form.InputPath, Model: form.Model, Format: domain.FormatMD}, nil
		},
		Runner: fakeRunner{},
	}, Options{
		In:          strings.NewReader("enter\nset input=input.m4a\nenter\nenter\nq\n"),
		Out:         &out,
		AllowNonTTY: true,
		Width:       40,
		Height:      12,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "out.md") {
		t.Fatalf("rendered = %q, want artifact", out.String())
	}
}

func TestRunScriptBuildSpecErrorMovesToError(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) {
			return domain.JobSpec{}, errors.New("bad spec")
		},
		Runner: fakeRunner{},
	}, Options{
		Out:         &out,
		AllowNonTTY: true,
		Script:      []tuiapp.Event{tuiapp.Submit{}, tuiapp.Submit{}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "bad spec") {
		t.Fatalf("rendered = %q, want bad spec", out.String())
	}
}

func TestTranslateLineCommands(t *testing.T) {
	cases := []string{"up", "down", "enter", "start", "esc", "set file=input.m4a"}
	for _, tc := range cases {
		events, _ := translateLine(tc)
		if len(events) == 0 {
			t.Fatalf("translateLine(%q) returned no events", tc)
		}
	}
	_, quit := translateLine("quit")
	if !quit {
		t.Fatal("quit should request loop exit")
	}
}

func TestTeaModelRunsProbeEffect(t *testing.T) {
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(form tuiapp.JobFormState) (domain.JobSpec, error) {
			if form.InputPath != "input.m4a" {
				t.Fatalf("InputPath = %q, want input.m4a", form.InputPath)
			}
			return domain.JobSpec{JobID: "job", InputPath: form.InputPath}, nil
		},
		Runner: fakeRunner{},
		Keys:   testKeyResolver(),
	}, Options{Width: 80, Height: 24})

	var cmd tea.Cmd
	model, cmd = model.apply(tuiapp.Submit{})
	if cmd != nil {
		t.Fatal("menu submit should not produce async command")
	}
	model, cmd = model.apply(tuiapp.InputChanged{Field: "input", Value: "input.m4a"})
	if cmd != nil {
		t.Fatal("input change should not produce async command")
	}
	model, cmd = model.apply(tuiapp.Submit{})
	if cmd == nil {
		t.Fatal("job submit should produce probe command")
	}
	msg := cmd()
	next, _ := model.Update(msg)
	updated := next.(teaModel)
	if updated.state.Screen != tuiapp.ScreenConfirm || !updated.state.Confirm.ProbeReady {
		t.Fatalf("state = %#v, want probe-ready confirm", updated.state)
	}
}

func TestTeaModelKeyEnterEditsSelectedJobField(t *testing.T) {
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Keys:      testKeyResolver(),
	}, Options{Width: 80, Height: 24})
	model.state.Screen = tuiapp.ScreenJobForm
	model.state.JobForm.Cursor = 1

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if model.state.Screen != tuiapp.ScreenEdit || model.state.Edit.Field != "input" {
		t.Fatalf("edit state/input = %#v/%q", model.state, model.input.Value())
	}
	model.input.SetValue("input.m4a")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if model.state.Screen != tuiapp.ScreenJobForm || model.state.JobForm.InputPath != "input.m4a" {
		t.Fatalf("state = %#v, want edited input", model.state)
	}
}

func TestTeaModelJobModelUsesSelectScreen(t *testing.T) {
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Keys:      testKeyResolver(),
	}, Options{Width: 80, Height: 24})
	model.state.Screen = tuiapp.ScreenJobForm
	model.state.JobForm.Cursor = 2

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if model.state.Screen != tuiapp.ScreenSelect || model.state.Select.Field != "model" {
		t.Fatalf("state = %#v, want model select", model.state)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = next.(teaModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if model.state.Screen != tuiapp.ScreenJobForm || model.state.JobForm.Model != "gpt-4o-mini-transcribe" {
		t.Fatalf("state = %#v, want selected model", model.state)
	}
}

func TestTeaModelCancelRunCallsCancel(t *testing.T) {
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Keys:      testKeyResolver(),
	}, Options{Width: 80, Height: 24})
	ctx, cancel := context.WithCancel(context.Background())
	model.cancelRun = cancel

	model, _ = model.applyEffects([]tuiapp.Effect{tuiapp.CancelRun{RequestID: "run"}})
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cancel was not called")
	}
}

func TestTeaModelClearsSecretInputOnEditCancel(t *testing.T) {
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Keys:      testKeyResolver(),
	}, Options{Width: 80, Height: 24})
	model.state.Screen = tuiapp.ScreenEdit
	model.state.APIKey.PendingMethod = "keychain"
	model.state.Edit.Password = true
	model.input.SetValue("sk-secret")

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(teaModel)

	if updated.state.Screen != tuiapp.ScreenAPIKey {
		t.Fatalf("Screen = %v, want APIKey", updated.state.Screen)
	}
	if updated.input.Value() != "" {
		t.Fatalf("input value leaked after cancel: %q", updated.input.Value())
	}
}

func TestTeaModelAPIKeyScreenSavesKeychain(t *testing.T) {
	store := &fakeSecretStore{}
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Config:    config.Default(),
		Keys: keystore.Resolver{
			Env:      fakeEnv{values: map[string]string{}},
			Keychain: store,
			Files:    &fakeFileStore{},
		},
	}, Options{Width: 80, Height: 24})
	model.state.Screen = tuiapp.ScreenAPIKey

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if model.state.Screen != tuiapp.ScreenEdit {
		t.Fatalf("Screen = %v, want edit", model.state.Screen)
	}
	model.input.SetValue("test-keychain-value")
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd()
	next, _ = model.Update(msg)
	model = next.(teaModel)

	if store.set != "test-keychain-value" {
		t.Fatalf("saved key = %q, want test-keychain-value", store.set)
	}
	if model.state.APIKey.CurrentStatus != "Registered (keychain)" {
		t.Fatalf("status = %q, want keychain", model.state.APIKey.CurrentStatus)
	}
}

func TestTeaModelAPIKeySaveRefreshesEffectiveSource(t *testing.T) {
	store := &fakeSecretStore{}
	model := newTeaModel(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Config:    config.Default(),
		Keys: keystore.Resolver{
			Env:      fakeEnv{values: map[string]string{"OPENAI_API_KEY": "sk-env"}},
			Keychain: store,
			Files:    &fakeFileStore{},
		},
	}, Options{Width: 80, Height: 24})
	model.state.Screen = tuiapp.ScreenAPIKey

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	model.input.SetValue("test-keychain-value")
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(teaModel)
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd()
	next, _ = model.Update(msg)
	model = next.(teaModel)

	if store.set != "test-keychain-value" {
		t.Fatalf("saved key = %q, want test-keychain-value", store.set)
	}
	if model.state.APIKey.ActiveSource != keystore.SourceOpenAIEnv || model.state.APIKey.CurrentStatus != "Registered (env override)" {
		t.Fatalf("api key state = %#v, want effective env source", model.state.APIKey)
	}
}

func TestRunScriptCanSaveFileAPIKey(t *testing.T) {
	files := &fakeFileStore{}
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		BuildSpec: func(tuiapp.JobFormState) (domain.JobSpec, error) { return domain.JobSpec{JobID: "job"}, nil },
		Runner:    fakeRunner{},
		Config:    config.Default(),
		Keys: keystore.Resolver{
			Env:      fakeEnv{values: map[string]string{}},
			Keychain: &fakeSecretStore{},
			Files:    files,
		},
	}, Options{
		Out:         &out,
		AllowNonTTY: true,
		Script: []tuiapp.Event{
			tuiapp.MoveDown{},
			tuiapp.Submit{},
			tuiapp.Submit{},
			tuiapp.MoveDown{},
			tuiapp.Submit{},
			tuiapp.EditSubmitted{Value: "sk-file"},
			tuiapp.ConfirmYes{},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if files.written != "sk-file" {
		t.Fatalf("written key = %q, want sk-file", files.written)
	}
	if !strings.Contains(out.String(), "Registered (file)") {
		t.Fatalf("rendered = %q, want file status", out.String())
	}
}
