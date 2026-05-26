package parse

import (
	"os"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	const src = "print -r -- hello\nfor x in a b c; do print -r -- \"$x\"; done\n"
	if _, err := Parse(strings.NewReader(src), "valid.zsh"); err != nil {
		t.Fatalf("expected a valid parse, got error: %v", err)
	}
}

func TestParseError(t *testing.T) {
	const src = "if true; then\n" // unterminated: missing `fi`
	if _, err := Parse(strings.NewReader(src), "invalid.zsh"); err == nil {
		t.Fatal("expected a parse error for an unterminated `if`, got nil")
	}
}

func TestParseFixture(t *testing.T) {
	f, err := os.Open("testdata/sample.zsh")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	if _, err := Parse(f, "sample.zsh"); err != nil {
		t.Fatalf("expected fixture to parse, got error: %v", err)
	}
}
