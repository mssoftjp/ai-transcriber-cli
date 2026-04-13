package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ai-transcriber-cli/internal/buildinfo"
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
