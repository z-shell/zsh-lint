package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Zsh lexes `\r` as an ordinary word character rather than whitespace, so a
// CRLF file is a parse error wherever a word may not appear: after `}`,
// `done`, `fi`, `esac` or `)`. mvdan/sh skips `\r` as whitespace and accepts
// the file, which made the linter report success for scripts Zsh refuses to
// run (#333). Every source here was checked against `zsh -f -n` as a file.
func TestParseRejectsCarriageReturnZshRejects(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line uint
		col  uint
	}{
		{name: "close brace", src: "f() { print x }\r\n", line: 1, col: 16},
		{name: "done", src: "while true; do :; done\r\n", line: 1, col: 23},
		{name: "fi", src: "if true; then :; fi\r\n", line: 1, col: 20},
		{name: "esac", src: "case x in y) :;; esac\r\n", line: 1, col: 22},
		{name: "subshell", src: "(true)\r\n", line: 1, col: 7},
		// A space before the `\r` does not help: the `\r` is still a word
		// where Zsh allows none.
		{name: "space before cr", src: "f() { print x } \r\n", line: 1, col: 17},
		// The `\r` need not be the first one in the file; the guard reports
		// the first `\r`, which is the byte Zsh stops at.
		{name: "second line", src: "print ok\r\nwhile true; do :; done\r\n", line: 1, col: 9},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, []byte(test.src), "parse error near `\\r`", test.line, test.col)
		})
	}
}

// The rule is not "a file containing `\r` is invalid". Zsh accepts a `\r`
// wherever a word may appear, and rejecting those would trade a false accept
// for a false reject. Every source here is accepted by `zsh -f -n`.
func TestParseAcceptsCarriageReturnZshAccepts(t *testing.T) {
	sources := []string{
		"x=1\r\n",
		"print x\r\n",
		"print x;\r\n",
		"true &&\r\ntrue\r\n",
		"print x |\r\nprint y\r\n",
		"print 'a\r'\n",
		"while true;\r\ndo :; done\n",
		"if true; then :; elif true\r\nthen :; fi\n",
		// No `\r` at all: the guard must not disturb ordinary sources.
		"f() { print x }\n",
	}

	for _, src := range sources {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// The invalid fixtures are `.txt` so the repository-wide `zsh -n` gate never
// sees them, per the parser-gap workflow.
func TestParseRejectsCarriageReturnFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/invalid-333-*.txt")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no invalid-333-* fixtures found")
	}

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			fixture, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if !strings.Contains(string(fixture), "\r") {
				t.Fatalf("fixture %s must contain a carriage return", path)
			}
			if err := parseString(t, string(fixture)); err == nil {
				t.Fatalf("fixture %s must stay rejected", path)
			}
		})
	}
}
