package buildinfo

import "testing"

func TestCurrent(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = oldVersion, oldCommit, oldDate
	})

	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-04-13T00:00:00Z"

	info := Current()
	if info.Version != "v1.2.3" {
		t.Fatalf("Version = %q, want v1.2.3", info.Version)
	}
	if info.Commit != "abc123" {
		t.Fatalf("Commit = %q, want abc123", info.Commit)
	}
	if info.Date != "2026-04-13T00:00:00Z" {
		t.Fatalf("Date = %q, want timestamp", info.Date)
	}
	if info.GoVersion == "" {
		t.Fatal("GoVersion is empty")
	}
}
