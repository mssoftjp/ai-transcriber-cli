//go:build darwin && cgo

package keystore

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSystemKeychainRoundTrip(t *testing.T) {
	if os.Getenv("TRANSCRIBER_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("set TRANSCRIBER_KEYCHAIN_INTEGRATION=1 to exercise macOS Keychain")
	}

	store := SystemKeychain{}
	service := "ai-transcriber-cli-test"
	account := fmt.Sprintf("openai-test-%d", time.Now().UnixNano())
	value := "sk-test-secret"
	t.Cleanup(func() {
		_ = store.Delete(service, account)
	})

	if err := store.Set(service, account, value); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := store.Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != value {
		t.Fatalf("Get() = %q, want %q", got, value)
	}
	if err := store.Delete(service, account); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(service, account); err != ErrNotFound {
		t.Fatalf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func TestSystemKeychainConfiguredItemReadable(t *testing.T) {
	if os.Getenv("TRANSCRIBER_KEYCHAIN_EXISTING") != "1" {
		t.Skip("set TRANSCRIBER_KEYCHAIN_EXISTING=1 to check the configured ai-transcriber-cli item")
	}

	got, err := SystemKeychain{}.Get(keychainService, keychainAccount)
	if err != nil {
		t.Fatalf("Get(%q, %q) error = %T %v", keychainService, keychainAccount, err, err)
	}
	if got == "" {
		t.Fatal("Get() returned an empty key")
	}
}
