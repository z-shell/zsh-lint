package parse

import (
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestCaseSeparatorBeforeIn(t *testing.T) {
	for _, src := range []string{
		"case $x; in a) print a ;; esac",
		"case $x ; in a) print a ;; esac",
		"case $x; ; in a) print a ;; esac",
		"case $x ; ; in a) print a ;; esac",
		"case $x ; ; ; in a) print a ;; esac",
		"case $x; ; ; ; in a) print a ;; esac",
		"case $x;\n; in a) print a ;; esac",
		"case $x\n;\nin a) print a ;; esac",
		"case $x ;\n;\nin a) print a ;; esac",
		"case $x ;\nin a) print a ;; esac",
		"case $x;\nin\na) print a ;; esac",
		"case $x; # note\nin a) print a ;; esac",
		"case $x; # note\n; in a) print a ;; esac",
		"case $x; in esac",
		"case $x\nin a) print a ;; esac",
		"case $x in a) print a ;; esac",
	} {
		_, err := Parse(strings.NewReader(src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestCaseSeparatorBeforeInInvalid(t *testing.T) {
	for _, src := range []string{
		"case $x;; in a) print a ;; esac",
		"case $x &; in a) print a ;; esac",
		"case $x & in a) print a ;; esac",
		"case $x | in a) print a ;; esac",
		"case $x || in a) print a ;; esac",
		"case $x && in a) print a ;; esac",
		"case ; in a) print a ;; esac",
		"case in a) print a ;; esac",
		"case $x ;;; in a) print a ;; esac",
		"case $x ;;\n; in a) print a ;; esac",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	src, err := os.ReadFile("testdata/invalid-485-case-double-separator.txt")
	if err != nil {
		t.Fatal(err)
	}
	// `;;` after the case word is one token, not two separators, so the
	// fork keeps its `in` error at the `case` keyword.
	var parseErr syntax.ParseError
	if _, err := Parse(strings.NewReader(string(src)), "invalid-485.zsh"); !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
	if got, want := parseErr.Pos.String(), "2:1"; got != want {
		t.Errorf("position = %s, want %s", got, want)
	}
	if got, want := parseErr.Text, "`case x` must be followed by `in`"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}
