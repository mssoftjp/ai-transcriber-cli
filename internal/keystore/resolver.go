package keystore

import (
	"errors"
	"strings"

	"ai-transcriber-cli/internal/config"
)

const (
	SourceOpenAIEnv     = "openai_env"
	SourceConfiguredEnv = "configured_env"
	SourceKeychain      = "keychain"
	SourceFile          = "file"
	SourceNone          = "none"
)

type ResolveResult struct {
	Value       string
	Source      string
	OverrideEnv bool
	EnvName     string
}

type EnvReader interface {
	Get(key string) string
}

type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Delete(service, account string) error
}

type FileStore interface {
	Path(cfg config.AppConfig) string
	Read(cfg config.AppConfig) (string, error)
	Write(cfg config.AppConfig, value string) error
	Delete(cfg config.AppConfig) error
}

type Resolver struct {
	Env      EnvReader
	Keychain Store
	Files    FileStore
}

func (r Resolver) ResolveAPIKey(cfg config.AppConfig) (ResolveResult, error) {
	envName := strings.TrimSpace(cfg.API.KeyEnv)
	if envName == "" {
		envName = "OPENAI_API_KEY"
	}
	if envName != "OPENAI_API_KEY" {
		if value := strings.TrimSpace(r.env().Get(envName)); value != "" {
			return ResolveResult{Value: value, Source: SourceConfiguredEnv, EnvName: envName}, nil
		}
	}
	if value := strings.TrimSpace(r.env().Get("OPENAI_API_KEY")); value != "" {
		return ResolveResult{Value: value, Source: SourceOpenAIEnv, OverrideEnv: envName != "OPENAI_API_KEY", EnvName: "OPENAI_API_KEY"}, nil
	}
	if r.Keychain != nil {
		value, err := r.Keychain.Get(keychainService, keychainAccount)
		if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrUnavailable) {
			return ResolveResult{}, err
		}
		if strings.TrimSpace(value) != "" {
			return ResolveResult{Value: strings.TrimSpace(value), Source: SourceKeychain, EnvName: envName}, nil
		}
	}
	if r.Files != nil {
		value, err := r.Files.Read(cfg)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return ResolveResult{}, err
		}
		if strings.TrimSpace(value) != "" {
			return ResolveResult{Value: strings.TrimSpace(value), Source: SourceFile, EnvName: envName}, nil
		}
	}
	return ResolveResult{Source: SourceNone, EnvName: envName}, nil
}

func (r Resolver) SaveAPIKey(cfg config.AppConfig, method string, value string) error {
	switch method {
	case SourceKeychain:
		if r.Keychain == nil {
			return ErrUnavailable
		}
		return r.Keychain.Set(keychainService, keychainAccount, value)
	case SourceFile:
		if r.Files == nil {
			return ErrUnavailable
		}
		return r.Files.Write(cfg, value)
	default:
		return ErrUnsupportedMethod
	}
}

func (r Resolver) DeleteAPIKey(cfg config.AppConfig, method string) error {
	switch method {
	case SourceKeychain:
		if r.Keychain == nil {
			return ErrUnavailable
		}
		return r.Keychain.Delete(keychainService, keychainAccount)
	case SourceFile:
		if r.Files == nil {
			return ErrUnavailable
		}
		return r.Files.Delete(cfg)
	default:
		return ErrUnsupportedMethod
	}
}

func (r Resolver) env() EnvReader {
	if r.Env != nil {
		return r.Env
	}
	return OSEnv{}
}

const (
	keychainService = "ai-transcriber-cli"
	keychainAccount = "openai"
)

var (
	ErrNotFound          = errors.New("key not found")
	ErrUnavailable       = errors.New("keystore unavailable")
	ErrUnsupportedMethod = errors.New("unsupported keystore method")
)
