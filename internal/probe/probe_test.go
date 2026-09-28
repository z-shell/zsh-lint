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

func TestReadRowsAndRowFiles(t *testing.T) {
	input := "# comment\n\nprint $x[(r)a[]\n  x=1  \n#\nprint \"#\"\n"
	rows, err := ReadRows(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"print $x[(r)a[]", "  x=1  ", "print \"#\""}
	if len(rows) != len(want) {
		t.Fatalf("ReadRows = %q, want %q", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
	files := RowFiles(make([]string, 10))
	if files[0].Name != "r01.zsh" || files[9].Name != "r10.zsh" || files[0].Content != "\n" {
		t.Fatalf("names = %q ... %q, content %q", files[0].Name, files[9].Name, files[0].Content)
	}
	if files := RowFiles(rows); files[1].Content != "  x=1  \n" {
		t.Fatalf("content = %q", files[1].Content)
	}

	// A row longer than the scanner's initial 64 KiB buffer is still read.
	long := "print " + strings.Repeat("a", 100*1024)
	rows, err = ReadRows(strings.NewReader(long + "\n"))
	if err != nil || len(rows) != 1 || rows[0] != long {
		t.Fatalf("long row: %d rows, %v", len(rows), err)
	}
}

// The #538 row file is the grid PR #539 reported: 9060 unique rows (#545).
// Its verdicts need builds of #539 and its base, which a shallow checkout
// does not hold; parser-gap-workflow.md records the run that reproduces them.
func TestRows538IsTheReportedGrid(t *testing.T) {
	source, err := os.Open(filepath.Join("testdata", "rows-538.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	rows, err := ReadRows(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9060 {
		t.Fatalf("rows-538.txt holds %d rows, want the 9060 PR #539 reported", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row] {
			t.Fatalf("duplicate row %q", row)
		}
		seen[row] = true
	}
}
