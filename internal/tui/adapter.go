package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/keystore"
	"ai-transcriber-cli/internal/plan"
	"ai-transcriber-cli/internal/tuiapp"
	"ai-transcriber-cli/internal/tuiview"
)

type SpecBuilder func(tuiapp.JobFormState) (domain.JobSpec, error)

type Runner interface {
	Probe(ctx context.Context, spec domain.JobSpec) (domain.ProbeResult, plan.ExecutionPlan, error)
	Transcribe(ctx context.Context, spec domain.JobSpec) ([]domain.Artifact, []byte, error)
}

type Options struct {
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	Width       int
	Height      int
	AllowNonTTY bool
	Script      []tuiapp.Event
}

type Dependencies struct {
	BuildSpec SpecBuilder
	Runner    Runner
	Config    config.AppConfig
	Keys      keystore.Resolver
}

func Run(ctx context.Context, deps Dependencies, opts Options) error {
	if deps.BuildSpec == nil {
		return errors.New("missing TUI spec builder")
	}
	if deps.Runner == nil {
		return errors.New("missing TUI runner")
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	if !opts.AllowNonTTY && !LooksInteractive(opts.In, opts.Out) {
		return domain.NewError("tui_not_supported", "transcriber tui requires an interactive terminal; use transcriber transcribe for non-interactive runs", domain.ExitArgs, nil)
	}

	state := tuiapp.InitialState()
	if len(opts.Script) > 0 {
		for _, event := range opts.Script {
			var err error
			state, err = step(ctx, deps, state, event)
			if errors.Is(err, errExitProgram) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintln(opts.Out, tuiview.Render(state, viewport(opts)))
		return nil
	}
	if LooksInteractive(opts.In, opts.Out) {
		return runBubbleTea(ctx, deps, opts)
	}

	scanner := bufio.NewScanner(opts.In)
	for {
		_, _ = fmt.Fprintln(opts.Out, tuiview.Render(state, viewport(opts)))
		_, _ = fmt.Fprint(opts.Out, "\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		events, quit := eventsForEnter(state, scanner.Text())
		for _, event := range events {
			var err error
			state, err = step(ctx, deps, state, event)
			if errors.Is(err, errExitProgram) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		if quit {
			return nil
		}
	}
}

type teaModel struct {
	ctx       context.Context
	deps      Dependencies
	state     tuiapp.State
	input     textinput.Model
	width     int
	height    int
	cancelRun context.CancelFunc
	err       error
}

type appEventMsg struct {
	event tuiapp.Event
}

func newTeaModel(ctx context.Context, deps Dependencies, opts Options) teaModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "value"
	input.CharLimit = 2048
	input.Blur()
	state := tuiapp.InitialState()
	state = applyAPIKeyStatusToState(state, deps)
	return teaModel{
		ctx:    ctx,
		deps:   deps,
		state:  state,
		input:  input,
		width:  viewport(opts).Width,
		height: viewport(opts).Height,
	}
}

func runBubbleTea(ctx context.Context, deps Dependencies, opts Options) error {
	lipgloss.SetColorProfile(termenv.ANSI256)
	model := newTeaModel(ctx, deps, opts)
	programOpts := []tea.ProgramOption{
		tea.WithInput(opts.In),
		tea.WithOutput(opts.Out),
		tea.WithAltScreen(),
	}
	p := tea.NewProgram(model, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(teaModel); ok && m.err != nil {
		return m.err
	}
	return nil
}

func (m teaModel) Init() tea.Cmd {
	return nil
}

func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m = m.syncInputMode()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case appEventMsg:
		return m.apply(msg.event)
	case tea.KeyMsg:
		action := translateKey(m.state.Screen, msg)
		switch {
		case action.quit:
			m.input.SetValue("")
			return m, tea.Quit
		case action.submitInput:
			value := m.input.Value()
			m.input.SetValue("")
			return m.apply(tuiapp.EditSubmitted{Value: value})
		case action.event != nil:
			return m.apply(action.event)
		case action.editInput:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}

func (m teaModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	contentWidth := max(1, width-2)
	body := tuiview.Render(m.state, tuiview.Viewport{Width: contentWidth, Height: max(1, m.height-3)})
	if m.state.Screen != tuiapp.ScreenEdit {
		return appStyle.Render(body)
	}
	return appStyle.Render(body) + "\n\n" + m.input.View()
}

func (m teaModel) apply(event tuiapp.Event) (teaModel, tea.Cmd) {
	oldScreen := m.state.Screen
	next, effects := tuiapp.Update(m.state, event)
	if oldScreen != tuiapp.ScreenEdit && next.Screen == tuiapp.ScreenEdit {
		m.input.SetValue(next.Edit.Value)
	}
	if oldScreen == tuiapp.ScreenEdit && next.Screen != tuiapp.ScreenEdit {
		m.input.SetValue("")
	}
	m.state = next
	return m.applyEffects(effects)
}

func (m teaModel) applyEffects(effects []tuiapp.Effect) (teaModel, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(effects))
	for _, effect := range effects {
		switch e := effect.(type) {
		case tuiapp.RunProbe:
			form := m.state.JobForm
			cmds = append(cmds, func() tea.Msg {
				spec, err := m.deps.BuildSpec(form)
				if err != nil {
					return appEventMsg{event: tuiapp.ProbeFailed{RequestID: e.RequestID, Err: err}}
				}
				probe, execPlan, err := m.deps.Runner.Probe(m.ctx, spec)
				if err != nil {
					return appEventMsg{event: tuiapp.ProbeFailed{RequestID: e.RequestID, Err: err}}
				}
				message := fmt.Sprintf("duration %.1fs, chunking %s", probe.DurationSec, execPlan.ChunkingMode)
				return appEventMsg{event: tuiapp.ProbeCompleted{RequestID: e.RequestID, Message: message}}
			})
		case tuiapp.RunTranscription:
			form := m.state.JobForm
			runCtx, cancel := context.WithCancel(m.ctx)
			m.cancelRun = cancel
			progressed, _ := tuiapp.Update(m.state, tuiapp.TranscriptionProgressed{
				RequestID: e.RequestID,
				Completed: 0,
				Total:     1,
				Stage:     domain.StageTranscribing,
				Message:   "transcribing",
			})
			m.state = progressed
			cmds = append(cmds, func() tea.Msg {
				spec, err := m.deps.BuildSpec(form)
				if err != nil {
					return appEventMsg{event: tuiapp.TranscriptionFailed{RequestID: e.RequestID, Err: err}}
				}
				artifacts, _, err := m.deps.Runner.Transcribe(runCtx, spec)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return appEventMsg{event: tuiapp.TranscriptionCancelled(e)}
					}
					return appEventMsg{event: tuiapp.TranscriptionFailed{RequestID: e.RequestID, Err: err}}
				}
				return appEventMsg{event: tuiapp.TranscriptionCompleted{RequestID: e.RequestID, Artifacts: artifacts}}
			})
		case tuiapp.CancelRun:
			if m.cancelRun != nil {
				m.cancelRun()
			}
		case tuiapp.SaveAPIKey:
			cmds = append(cmds, func() tea.Msg {
				resolver := m.keys()
				err := resolver.SaveAPIKey(m.deps.Config, e.Method, e.Value)
				if err != nil {
					return appEventMsg{event: tuiapp.APIKeySaveFailed{Err: err}}
				}
				return appEventMsg{event: apiKeyStatusEvent(m.deps.Config, resolver, "API key saved")}
			})
		case tuiapp.DeleteAPIKey:
			cmds = append(cmds, func() tea.Msg {
				resolver := m.keys()
				err := resolver.DeleteAPIKey(m.deps.Config, e.Source)
				if err != nil && !errors.Is(err, keystore.ErrNotFound) {
					return appEventMsg{event: tuiapp.APIKeyDeleteFailed{Err: err}}
				}
				return appEventMsg{event: apiKeyStatusEvent(m.deps.Config, resolver, "Saved key deleted")}
			})
		case tuiapp.ExitProgram:
			cmds = append(cmds, tea.Quit)
		}
	}
	return m.syncInputMode(), tea.Batch(cmds...)
}

func (m teaModel) keys() keystore.Resolver {
	if m.deps.Keys.Env == nil && m.deps.Keys.Keychain == nil && m.deps.Keys.Files == nil {
		return keystore.Resolver{Env: keystore.OSEnv{}, Keychain: keystore.SystemKeychain{}, Files: keystore.DefaultFileStore{}}
	}
	return m.deps.Keys
}

func (m teaModel) syncInputMode() teaModel {
	if m.state.Screen == tuiapp.ScreenEdit && m.state.Edit.Password {
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "paste API key"
		_ = m.input.Focus()
		return m
	}
	if m.state.Screen == tuiapp.ScreenEdit {
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "value"
		_ = m.input.Focus()
		return m
	}
	m.input.EchoMode = textinput.EchoNormal
	m.input.Placeholder = "value"
	m.input.Blur()
	return m
}

var (
	appStyle = lipgloss.NewStyle().Padding(0, 1)
)

func step(ctx context.Context, deps Dependencies, state tuiapp.State, event tuiapp.Event) (tuiapp.State, error) {
	next, effects := tuiapp.Update(state, event)
	for _, effect := range effects {
		var err error
		next, err = runEffect(ctx, deps, next, effect)
		if err != nil {
			return next, err
		}
	}
	return next, nil
}

func runEffect(ctx context.Context, deps Dependencies, state tuiapp.State, effect tuiapp.Effect) (tuiapp.State, error) {
	switch e := effect.(type) {
	case tuiapp.RunProbe:
		spec, err := deps.BuildSpec(state.JobForm)
		if err != nil {
			next, _ := tuiapp.Update(state, tuiapp.ProbeFailed{RequestID: e.RequestID, Err: err})
			return next, nil
		}
		probe, execPlan, err := deps.Runner.Probe(ctx, spec)
		if err != nil {
			next, _ := tuiapp.Update(state, tuiapp.ProbeFailed{RequestID: e.RequestID, Err: err})
			return next, nil
		}
		message := fmt.Sprintf("duration %.1fs, chunking %s", probe.DurationSec, execPlan.ChunkingMode)
		next, _ := tuiapp.Update(state, tuiapp.ProbeCompleted{RequestID: e.RequestID, Message: message})
		return next, nil
	case tuiapp.RunTranscription:
		spec, err := deps.BuildSpec(state.JobForm)
		if err != nil {
			next, _ := tuiapp.Update(state, tuiapp.TranscriptionFailed{RequestID: e.RequestID, Err: err})
			return next, nil
		}
		progress, _ := tuiapp.Update(state, tuiapp.TranscriptionProgressed{
			RequestID: e.RequestID,
			Completed: 0,
			Total:     1,
			Stage:     domain.StageTranscribing,
			Message:   "transcribing",
		})
		artifacts, _, err := deps.Runner.Transcribe(ctx, spec)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				next, _ := tuiapp.Update(progress, tuiapp.TranscriptionCancelled(e))
				return next, nil
			}
			next, _ := tuiapp.Update(progress, tuiapp.TranscriptionFailed{RequestID: e.RequestID, Err: err})
			return next, nil
		}
		next, _ := tuiapp.Update(progress, tuiapp.TranscriptionCompleted{RequestID: e.RequestID, Artifacts: artifacts})
		return next, nil
	case tuiapp.ExitProgram:
		return state, errExitProgram
	case tuiapp.SaveAPIKey:
		resolver := deps.Keys
		if resolver.Env == nil && resolver.Keychain == nil && resolver.Files == nil {
			resolver = keystore.Resolver{Env: keystore.OSEnv{}, Keychain: keystore.SystemKeychain{}, Files: keystore.DefaultFileStore{}}
		}
		if err := resolver.SaveAPIKey(deps.Config, e.Method, e.Value); err != nil {
			next, _ := tuiapp.Update(state, tuiapp.APIKeySaveFailed{Err: err})
			return next, nil
		}
		next, _ := tuiapp.Update(state, apiKeyStatusEvent(deps.Config, resolver, "API key saved"))
		return next, nil
	case tuiapp.DeleteAPIKey:
		resolver := deps.Keys
		if resolver.Env == nil && resolver.Keychain == nil && resolver.Files == nil {
			resolver = keystore.Resolver{Env: keystore.OSEnv{}, Keychain: keystore.SystemKeychain{}, Files: keystore.DefaultFileStore{}}
		}
		if err := resolver.DeleteAPIKey(deps.Config, e.Source); err != nil && !errors.Is(err, keystore.ErrNotFound) {
			next, _ := tuiapp.Update(state, tuiapp.APIKeyDeleteFailed{Err: err})
			return next, nil
		}
		next, _ := tuiapp.Update(state, apiKeyStatusEvent(deps.Config, resolver, "Saved key deleted"))
		return next, nil
	default:
		return state, nil
	}
}

var errExitProgram = errors.New("exit TUI")

func translateLine(line string) ([]tuiapp.Event, bool) {
	value := strings.TrimSpace(line)
	switch value {
	case "q", "quit":
		return []tuiapp.Event{tuiapp.Cancel{}}, true
	case "up", "k":
		return []tuiapp.Event{tuiapp.MoveUp{}}, false
	case "down", "j":
		return []tuiapp.Event{tuiapp.MoveDown{}}, false
	case "", "enter":
		return []tuiapp.Event{tuiapp.Submit{}}, false
	case "start":
		return []tuiapp.Event{tuiapp.StartRun{}}, false
	case "esc", "back", "cancel":
		return []tuiapp.Event{tuiapp.Cancel{}}, false
	}
	if strings.HasPrefix(value, "set ") {
		field, val, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(value, "set ")), "=")
		if ok {
			return []tuiapp.Event{tuiapp.InputChanged{Field: normalizeField(field), Value: strings.TrimSpace(val)}}, false
		}
	}
	return nil, false
}

func eventsForEnter(state tuiapp.State, value string) ([]tuiapp.Event, bool) {
	trimmed := strings.TrimSpace(value)
	if state.Screen == tuiapp.ScreenEdit {
		return []tuiapp.Event{tuiapp.EditSubmitted{Value: trimmed}}, false
	}
	if state.Screen == tuiapp.ScreenConfirmLocalFile || state.Screen == tuiapp.ScreenConfirmDeleteAPIKey {
		switch strings.ToLower(trimmed) {
		case "", "y", "yes", "enter":
			return []tuiapp.Event{tuiapp.ConfirmYes{}}, false
		case "n", "no", "esc", "cancel":
			return []tuiapp.Event{tuiapp.ConfirmNo{}}, false
		}
	}
	return translateLine(value)
}

func applyAPIKeyStatusToState(state tuiapp.State, deps Dependencies) tuiapp.State {
	resolver := deps.Keys
	if resolver.Env == nil && resolver.Keychain == nil && resolver.Files == nil {
		resolver = keystore.Resolver{Env: keystore.OSEnv{}, Keychain: keystore.SystemKeychain{}, Files: keystore.DefaultFileStore{}}
	}
	next, _ := tuiapp.Update(state, apiKeyStatusEvent(deps.Config, resolver, ""))
	return next
}

func apiKeyStatusEvent(cfg config.AppConfig, resolver keystore.Resolver, status string) tuiapp.APIKeyStatusChanged {
	event := tuiapp.APIKeyStatusChanged{
		Method:  "auto",
		EnvName: valueOr(cfg.API.KeyEnv, "OPENAI_API_KEY"),
		Status:  status,
	}
	if resolver.Files != nil {
		event.FilePath = resolver.Files.Path(cfg)
	}
	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		event.CurrentStatus = "Check failed"
		event.Status = err.Error()
	} else {
		event.ActiveSource = result.Source
		event.CurrentStatus = apiKeyStatusLabel(result.Source)
		event.EnvName = valueOr(result.EnvName, event.EnvName)
	}
	if resolver.Files != nil {
		if warning := filePermissionWarning(resolver.Files.Path(cfg)); warning != "" {
			event.FileWarning = warning
		}
	}
	return event
}

func apiKeyStatusLabel(source string) string {
	switch source {
	case keystore.SourceOpenAIEnv:
		return "Registered (env override)"
	case keystore.SourceConfiguredEnv:
		return "Registered (configured env)"
	case keystore.SourceKeychain:
		return "Registered (keychain)"
	case keystore.SourceFile:
		return "Registered (file)"
	default:
		return "Not configured"
	}
}

func filePermissionWarning(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Sprintf("local key file permissions are %04o; expected 0600", info.Mode().Perm())
	}
	return ""
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func normalizeField(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "input", "input file", "file":
		return "input"
	case "model":
		return "model"
	case "format":
		return "format"
	case "language", "lang":
		return "language"
	case "output", "out", "output path":
		return "output"
	case "prompt":
		return "prompt"
	default:
		return field
	}
}

func viewport(opts Options) tuiview.Viewport {
	width := opts.Width
	if width == 0 {
		width = 80
	}
	height := opts.Height
	if height == 0 {
		height = 24
	}
	return tuiview.Viewport{Width: width, Height: height}
}

func LooksInteractive(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	return inOK && outOK && isCharDevice(inFile) && isCharDevice(outFile)
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
