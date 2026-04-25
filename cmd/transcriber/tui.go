package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"ai-transcriber-cli/internal/app"
	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
	"ai-transcriber-cli/internal/keystore"
	tuiadapter "ai-transcriber-cli/internal/tui"
	"ai-transcriber-cli/internal/tuiapp"
)

func newTUICmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Run the interactive terminal UI",
		Long: `Run an interactive terminal UI for a single transcription job.

The batch CLI remains the primary automation interface. Use transcriber transcribe,
probe, doctor, and config for scripts and redirected input/output.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !tuiadapter.LooksInteractive(os.Stdin, cmd.OutOrStdout()) {
				return domain.NewError("tui_not_supported", "transcriber tui requires an interactive terminal; use transcriber transcribe for non-interactive runs", domain.ExitArgs, nil)
			}
			resolver := defaultKeyResolver()
			key, err := resolver.ResolveAPIKey(state.cfg)
			if err != nil {
				key = keystore.ResolveResult{EnvName: valueOr(state.cfg.API.KeyEnv, "OPENAI_API_KEY")}
			}
			baseSpec, err := buildSpecFromForm(state.cfg, tuiapp.InitialState().JobForm)
			if err != nil {
				return err
			}
			services, err := buildCoreServices(baseSpec, key.Value, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			runner := &tuiRunner{
				Services:  services,
				cfg:       state.cfg,
				resolver:  resolver,
				stdout:    cmd.OutOrStdout(),
				stderr:    cmd.ErrOrStderr(),
				apiKey:    key.Value,
				apiKeyEnv: valueOr(key.EnvName, "OPENAI_API_KEY"),
			}
			return tuiadapter.Run(cmd.Context(), tuiadapter.Dependencies{
				BuildSpec: func(form tuiapp.JobFormState) (domain.JobSpec, error) {
					return buildSpecFromForm(state.cfg, form)
				},
				Runner: runner,
				Config: state.cfg,
				Keys:   resolver,
			}, tuiadapter.Options{
				In:     os.Stdin,
				Out:    cmd.OutOrStdout(),
				Err:    cmd.ErrOrStderr(),
				Width:  80,
				Height: 24,
			})
		},
	}
	return cmd
}

type tuiRunner struct {
	*app.Services
	cfg       config.AppConfig
	resolver  keystore.Resolver
	stdout    io.Writer
	stderr    io.Writer
	apiKey    string
	apiKeyEnv string
}

func (r *tuiRunner) Transcribe(ctx context.Context, spec domain.JobSpec) ([]domain.Artifact, []byte, error) {
	services, err := r.servicesForTranscribe(spec)
	if err != nil {
		return nil, nil, err
	}
	return services.Transcribe(ctx, spec)
}

func (r *tuiRunner) servicesForTranscribe(spec domain.JobSpec) (*app.Services, error) {
	key, err := r.resolver.ResolveAPIKey(r.cfg)
	if err != nil {
		return nil, domain.NewError("api_key_resolve_failed", "failed to resolve API key", domain.ExitAuth, err)
	}
	r.apiKeyEnv = valueOr(key.EnvName, "OPENAI_API_KEY")
	if strings.TrimSpace(key.Value) == "" {
		message := fmt.Sprintf("%s is required. Export it first, then re-run the command. Example: export %s=\"sk-...\"", r.apiKeyEnv, r.apiKeyEnv)
		return nil, domain.NewError("missing_api_key", message, domain.ExitAuth, nil)
	}
	if r.Services != nil && r.apiKey == key.Value {
		return r.Services, nil
	}
	services, err := buildCoreServices(spec, key.Value, r.stdout, r.stderr)
	if err != nil {
		return nil, err
	}
	r.Services = services
	r.apiKey = key.Value
	return services, nil
}

func buildSpecFromForm(cfg config.AppConfig, form tuiapp.JobFormState) (domain.JobSpec, error) {
	transcribeCmd := &cobra.Command{Use: "transcribe"}
	addCommonTranscribeFlags(transcribeCmd)
	setFlag(transcribeCmd, "model", form.Model)
	setFlag(transcribeCmd, "format", form.Format)
	setFlag(transcribeCmd, "language", form.Language)
	setFlag(transcribeCmd, "out", form.OutputPath)
	setFlag(transcribeCmd, "prompt", form.Prompt)
	setFlag(transcribeCmd, "events", string(domain.EventsNone))
	return buildSpec(cfg, transcribeCmd, form.InputPath)
}

func setFlag(cmd *cobra.Command, name, value string) {
	if value == "" {
		return
	}
	_ = cmd.Flags().Set(name, value)
}
