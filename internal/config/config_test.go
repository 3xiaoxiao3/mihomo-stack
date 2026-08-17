package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesPathsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardian.yaml")
	content := `
subscription:
  sources:
    - name: primary
      url_file: secrets/subscription
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Storage.DataDir != filepath.Join(dir, "data") {
		t.Fatalf("data dir = %q", cfg.Storage.DataDir)
	}
	if cfg.Subscription.Sources[0].URLFile != filepath.Join(dir, "secrets/subscription") {
		t.Fatalf("URL file = %q", cfg.Subscription.Sources[0].URLFile)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardian.yaml")
	content := `
unknown: true
subscription:
  sources:
    - name: primary
      url_file: subscription
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestRemoteListenerRequiresAuthentication(t *testing.T) {
	cfg := Defaults()
	cfg.Server.Listen = "0.0.0.0:8080"
	cfg.Subscription.Sources = []SubscriptionSource{{Name: "primary", URLFile: "/secret"}}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "remote listener requires") {
		t.Fatalf("expected remote-listener error, got %v", err)
	}
}

func TestReadSecretTrimsOneLineEnding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("token-value\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	value, err := ReadSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if value != "token-value" {
		t.Fatalf("secret = %q", value)
	}
}

func TestReadSecretRejectsMultipleLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err == nil || !strings.Contains(err.Error(), "exactly one line") {
		t.Fatalf("expected multiline error, got %v", err)
	}
}
