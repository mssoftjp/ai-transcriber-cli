package keystore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ai-transcriber-cli/internal/config"
)

type DefaultFileStore struct {
	ExplicitPath string
}

func (s DefaultFileStore) Path(config.AppConfig) string {
	if strings.TrimSpace(s.ExplicitPath) != "" {
		return s.ExplicitPath
	}
	return filepath.Join(filepath.Dir(config.DefaultPath()), "key.txt")
}

func (s DefaultFileStore) Read(cfg config.AppConfig) (string, error) {
	data, err := os.ReadFile(s.Path(cfg))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s DefaultFileStore) Write(cfg config.AppConfig, value string) error {
	path := s.Path(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)+"\n"), 0o600)
}

func (s DefaultFileStore) Delete(cfg config.AppConfig) error {
	err := os.Remove(s.Path(cfg))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
