package probe

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A context that is invalid Zsh on its own would turn every probe placed in
// it into a false signal, so each template must parse natively with a plain
// command in its slot. The verdict is stderr, not exit status (#404).
func TestContextsAreValidZsh(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required to check the contexts")
	}
	files, err := Generate([]string{"print probe"}, Contexts)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range files {
		path := filepath.Join(dir, file.Name)
		if err := os.WriteFile(path, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, zsh, "-f", "-n", path)
		cmd.Stderr = &stderr
		_ = cmd.Run()
		cancel()
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			t.Errorf("context %s is not valid Zsh with a plain command:\n%s\n%s", file.Name, file.Content, msg)
		}
	}
}

func TestContextNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, context := range Contexts {
		if seen[context.Name] {
			t.Errorf("duplicate context name %q", context.Name)
		}
		seen[context.Name] = true
		if strings.Count(context.Template, Slot) != 1 {
			t.Errorf("context %s must hold %s exactly once", context.Name, Slot)
		}
	}
}

func TestReadBodies(t *testing.T) {
	input := "\nfor x (a b) print $x\n---\n\n---\nrepeat 2 do\n  print a\ndone\n\n---\n"
	got, err := ReadBodies(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"for x (a b) print $x", "repeat 2 do\n  print a\ndone"}
	if len(got) != len(want) {
		t.Fatalf("ReadBodies = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("body %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGenerateIsDeterministicAndNamesBothHalves(t *testing.T) {
	bodies := make([]string, 12)
	for i := range bodies {
		bodies[i] = "print body"
	}
	first, err := Generate(bodies, Contexts)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Generate(bodies, Contexts)
	if len(first) != len(bodies)*len(Contexts) {
		t.Fatalf("generated %d files, want %d", len(first), len(bodies)*len(Contexts))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("file %d differs between runs", i)
		}
	}
	if first[0].Name != "b01-top.zsh" || first[len(first)-1].Name != "b12-"+Contexts[len(Contexts)-1].Name+".zsh" {
		t.Fatalf("names = %q ... %q", first[0].Name, first[len(first)-1].Name)
	}
	if _, err := Generate(bodies, []Context{{"broken", "no slot"}}); err == nil {
		t.Fatal("Generate accepted a context without a slot")
	}
}

func TestWriteRefusesAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, nil); err == nil {
		t.Fatal("Write reused an existing directory")
	}
	grid := filepath.Join(dir, "grid")
	if err := Write(grid, []File{{Name: "b1-top.zsh", Content: "print a\n"}}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(grid, "b1-top.zsh")); err != nil || string(got) != "print a\n" {
		t.Fatalf("written file = %q, %v", got, err)
	}
}
