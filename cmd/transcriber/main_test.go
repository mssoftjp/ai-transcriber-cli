package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-transcriber-cli/internal/buildinfo"
	"ai-transcriber-cli/internal/config"
	"ai-transcriber-cli/internal/domain"
)

func TestVersionCommandOutputs(t *testing.T) {
	oldVersion, oldCommit, oldDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = oldVersion, oldCommit, oldDate
	})
	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "abc123"
	buildinfo.Date = "2026-04-13T00:00:00Z"

	t.Run("json", func(t *testing.T) {
		var stdout bytes.Buffer
		root := buildRootForTest(&stdout, &bytes.Buffer{})
		root.SetArgs([]string{"version", "--json"})
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("ExecuteContext(): %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal(): %v", err)
		}
		if payload["cli_version"] != "v1.2.3" {
			t.Fatalf("cli_version = %#v, want v1.2.3", payload["cli_version"])
		}
	})

	t.Run("short", func(t *testing.T) {
		var stdout bytes.Buffer
		root := buildRootForTest(&stdout, &bytes.Buffer{})
		root.SetArgs([]string{"version", "--short"})
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("ExecuteContext(): %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "v1.2.3" {
			t.Fatalf("stdout = %q, want v1.2.3", got)
		}
	})
}

func TestTUIHelpIsAvailable(t *testing.T) {
	var stdout bytes.Buffer
	root := buildRootForTest(&stdout, &bytes.Buffer{})
	root.SetArgs([]string{"tui", "--help"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(): %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "interactive terminal UI") {
		t.Fatalf("stdout = %q, want TUI help", got)
	}
}

func TestTUIRejectsNonInteractiveIO(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := buildRootForTest(&stdout, &stderr)
	root.SetArgs([]string{"tui"})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected non-interactive TUI error")
	}
	if code := domain.ErrorCode(err); code != "tui_not_supported" {
		t.Fatalf("error code = %q, want tui_not_supported", code)
	}
}

func TestConfigKeyStatusDoesNotPrintSecret(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-secret")
	var stdout bytes.Buffer
	root := buildRootForTest(&stdout, &bytes.Buffer{})
	root.SetArgs([]string{"config", "key", "status"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(): %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "sk-test-secret") {
		t.Fatalf("status leaked secret: %q", got)
	}
	if !strings.Contains(got, `"found": true`) || !strings.Contains(got, `"source": "openai_env"`) {
		t.Fatalf("status = %q, want found openai_env", got)
	}
}

func TestConfigKeyStatusPrefersConfiguredEnvOverOpenAIEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-secret")
	t.Setenv("CUSTOM_OPENAI_KEY", "sk-custom-secret")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[api]\nkey_env = \"CUSTOM_OPENAI_KEY\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	var stdout bytes.Buffer
	root := buildRootForTest(&stdout, &bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "config", "key", "status"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(): %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "sk-openai-secret") || strings.Contains(got, "sk-custom-secret") {
		t.Fatalf("status leaked secret: %q", got)
	}
	if !strings.Contains(got, `"source": "configured_env"`) || !strings.Contains(got, `"env_name": "CUSTOM_OPENAI_KEY"`) {
		t.Fatalf("status = %q, want configured env", got)
	}
}

func TestKeyValueForSetReadsProvidedInput(t *testing.T) {
	got, err := keyValueForSet(config.Default(), "", true, strings.NewReader(" sk-from-reader \n"))
	if err != nil {
		t.Fatalf("keyValueForSet() error = %v", err)
	}
	if got != "sk-from-reader" {
		t.Fatalf("keyValueForSet() = %q, want reader value", got)
	}
}
