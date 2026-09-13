package projectconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFindsNearestAncestorConfiguration(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "pkg", "functions")
	filename := filepath.Join(nested, "example")
	want := filepath.Join(root, "pkg", "zsh-lint.json")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(want, []byte(validConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Discover(filename)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got != want {
		t.Fatalf("Discover() = %q, want %q", got, want)
	}
}

func TestDiscoverReturnsEmptyWhenNoConfigurationExists(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "script.zsh")

	got, err := Discover(filename)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Discover() = %q, want empty", got)
	}
}

func TestDiscoverAcceptsDirectoryInput(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "pkg", "functions")
	want := filepath.Join(nested, "zsh-lint.json")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(want, []byte(validConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Discover(nested)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got != want {
		t.Fatalf("Discover() = %q, want %q", got, want)
	}
}
