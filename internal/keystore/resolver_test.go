package keystore

import (
	"errors"
	"testing"

	"ai-transcriber-cli/internal/config"
)

type mapEnv map[string]string

func (m mapEnv) Get(key string) string { return m[key] }

type memoryFileStore struct {
	value string
	err   error
	path  string
}

type memoryKeychain struct {
	value string
	err   error
}

func (m *memoryKeychain) Get(string, string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.value == "" {
		return "", ErrNotFound
	}
	return m.value, nil
}

func (m *memoryKeychain) Set(_, _, value string) error {
	m.value = value
	return nil
}

func (m *memoryKeychain) Delete(string, string) error {
	m.value = ""
	return nil
}

func (m *memoryFileStore) Path(config.AppConfig) string {
	if m.path != "" {
		return m.path
	}
	return "/tmp/key.txt"
}
func (m *memoryFileStore) Read(config.AppConfig) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.value == "" {
		return "", ErrNotFound
	}
	return m.value, nil
}
func (m *memoryFileStore) Write(_ config.AppConfig, value string) error {
	m.value = value
	return nil
}
func (m *memoryFileStore) Delete(config.AppConfig) error {
	m.value = ""
	return nil
}

func TestResolveAPIKeyPrefersConfiguredEnvOverOpenAIEnv(t *testing.T) {
	cfg := config.Default()
	cfg.API.KeyEnv = "CUSTOM_KEY"
	resolver := Resolver{
		Env:   mapEnv{"OPENAI_API_KEY": "openai", "CUSTOM_KEY": "custom"},
		Files: &memoryFileStore{value: "file"},
	}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "custom" || result.Source != SourceConfiguredEnv || result.EnvName != "CUSTOM_KEY" {
		t.Fatalf("result = %#v, want configured env", result)
	}
}

func TestResolveAPIKeyFallsBackToOpenAIEnvWhenConfiguredEnvIsUnset(t *testing.T) {
	cfg := config.Default()
	cfg.API.KeyEnv = "CUSTOM_KEY"
	resolver := Resolver{
		Env:   mapEnv{"OPENAI_API_KEY": "openai"},
		Files: &memoryFileStore{value: "file"},
	}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "openai" || result.Source != SourceOpenAIEnv || !result.OverrideEnv {
		t.Fatalf("result = %#v, want openai env fallback", result)
	}
}

func TestResolveAPIKeyUsesConfiguredEnvBeforeFile(t *testing.T) {
	cfg := config.Default()
	cfg.API.KeyEnv = "CUSTOM_KEY"
	resolver := Resolver{
		Env:   mapEnv{"CUSTOM_KEY": "custom"},
		Files: &memoryFileStore{value: "file"},
	}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "custom" || result.Source != SourceConfiguredEnv || result.EnvName != "CUSTOM_KEY" {
		t.Fatalf("result = %#v, want configured env", result)
	}
}

func TestResolveAPIKeyFallsBackToFile(t *testing.T) {
	cfg := config.Default()
	resolver := Resolver{Env: mapEnv{}, Files: &memoryFileStore{value: "file"}}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "file" || result.Source != SourceFile {
		t.Fatalf("result = %#v, want file", result)
	}
}

func TestResolveAPIKeyPrefersKeychainBeforeFile(t *testing.T) {
	cfg := config.Default()
	resolver := Resolver{
		Env:      mapEnv{},
		Keychain: &memoryKeychain{value: "keychain"},
		Files:    &memoryFileStore{value: "file"},
	}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "keychain" || result.Source != SourceKeychain {
		t.Fatalf("result = %#v, want keychain", result)
	}
}

func TestResolveAPIKeySkipsUnavailableKeychain(t *testing.T) {
	cfg := config.Default()
	resolver := Resolver{
		Env:      mapEnv{},
		Keychain: &memoryKeychain{err: ErrUnavailable},
		Files:    &memoryFileStore{value: "file"},
	}

	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "file" || result.Source != SourceFile {
		t.Fatalf("result = %#v, want file fallback", result)
	}
}

func TestSaveAndDeleteAPIKeyInKeychain(t *testing.T) {
	cfg := config.Default()
	keys := &memoryKeychain{}
	resolver := Resolver{Env: mapEnv{}, Keychain: keys}

	if err := resolver.SaveAPIKey(cfg, SourceKeychain, "secret"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}
	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "secret" || result.Source != SourceKeychain {
		t.Fatalf("result = %#v, want keychain secret", result)
	}
	if err := resolver.DeleteAPIKey(cfg, SourceKeychain); err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}
	result, err = resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() after delete error = %v", err)
	}
	if result.Source != SourceNone {
		t.Fatalf("result = %#v, want none", result)
	}
}

func TestResolveAPIKeyReturnsFileErrors(t *testing.T) {
	cfg := config.Default()
	resolver := Resolver{Env: mapEnv{}, Files: &memoryFileStore{err: errors.New("boom")}}

	_, err := resolver.ResolveAPIKey(cfg)
	if err == nil {
		t.Fatal("expected file error")
	}
}

func TestSaveAndDeleteAPIKeyInFile(t *testing.T) {
	cfg := config.Default()
	files := &memoryFileStore{}
	resolver := Resolver{Env: mapEnv{}, Files: files}

	if err := resolver.SaveAPIKey(cfg, SourceFile, "secret"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}
	result, err := resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() error = %v", err)
	}
	if result.Value != "secret" || result.Source != SourceFile {
		t.Fatalf("result = %#v, want file secret", result)
	}
	if err := resolver.DeleteAPIKey(cfg, SourceFile); err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}
	result, err = resolver.ResolveAPIKey(cfg)
	if err != nil {
		t.Fatalf("ResolveAPIKey() after delete error = %v", err)
	}
	if result.Source != SourceNone {
		t.Fatalf("result = %#v, want none", result)
	}
}

func TestSaveAPIKeyRejectsUnsupportedMethod(t *testing.T) {
	err := (Resolver{}).SaveAPIKey(config.Default(), "env", "secret")
	if !errors.Is(err, ErrUnsupportedMethod) {
		t.Fatalf("error = %v, want unsupported method", err)
	}
}

func TestFileStoreReadWriteDelete(t *testing.T) {
	cfg := config.Default()
	store := DefaultFileStore{ExplicitPath: t.TempDir() + "/key.txt"}

	if _, err := store.Read(cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read missing error = %v, want ErrNotFound", err)
	}
	if err := store.Write(cfg, " secret "); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := store.Read(cfg)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "secret" {
		t.Fatalf("Read() = %q, want secret", got)
	}
	if err := store.Delete(cfg); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
