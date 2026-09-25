package parse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestHeredocAt(t *testing.T) {
	tests := []struct {
		src       string
		heredoc   pendingHeredoc
		end       int
		isHeredoc bool
		ok        bool
	}{
		{src: "<<EOF\n", heredoc: pendingHeredoc{delimiter: "EOF"}, end: 5, isHeredoc: true, ok: true},
		{src: "<<-EOF\n", heredoc: pendingHeredoc{delimiter: "EOF", stripTabs: true}, end: 6, isHeredoc: true, ok: true},
		{src: "<< 'EOF'\n", heredoc: pendingHeredoc{delimiter: "EOF"}, end: 8, isHeredoc: true, ok: true},
		{src: "<<<\"'\"\n", end: 3, isHeredoc: false, ok: true},
		{src: "< in\n", end: 0, isHeredoc: false, ok: true},
		{src: "<<\n", end: 0, isHeredoc: true, ok: false},
		{src: "<", end: 0, isHeredoc: false, ok: true},
		{src: "<<", end: 0, isHeredoc: true, ok: false},
		{src: "<<<", end: 3, isHeredoc: false, ok: true},
		{src: "<<-", end: 0, isHeredoc: true, ok: false},
		{src: "x<<EOF", end: 0, isHeredoc: false, ok: true},
	}
	for _, tt := range tests {
		heredoc, end, isHeredoc, ok := heredocAt([]byte(tt.src), 0)
		if heredoc != tt.heredoc || end != tt.end || isHeredoc != tt.isHeredoc || ok != tt.ok {
			t.Errorf("heredocAt(%q) = %+v, %d, %v, %v; want %+v, %d, %v, %v",
				tt.src, heredoc, end, isHeredoc, ok, tt.heredoc, tt.end, tt.isHeredoc, tt.ok)
		}
	}
}

func TestAtUnescapedLineStart(t *testing.T) {
	src := []byte("a\nb\\\nc\\\\\nd")
	for i, want := range map[int]bool{0: false, 1: false, 2: true, 5: false, 9: true} {
		if got := atUnescapedLineStart(src, i); got != want {
			t.Errorf("atUnescapedLineStart(%q, %d) = %v, want %v", src, i, got, want)
		}
	}
}

// A line of backslashes alone decides continuation by parity, including a
// run that starts at the beginning of the source.
func TestAtUnescapedLineStartBackslashRuns(t *testing.T) {
	for src, want := range map[string]bool{
		"\\\nx":        false,
		"\\\\\nx":      true,
		"\\\\\\\nx":    false,
		"a\\\\\\\\\nx": true,
	} {
		if got := atUnescapedLineStart([]byte(src), len(src)-1); got != want {
			t.Errorf("atUnescapedLineStart(%q, %d) = %v, want %v", src, len(src)-1, got, want)
		}
	}
}

// The alternate if/while and the for scanners skip here-document bodies and
// tell a here-string and an arithmetic shift from a here-document, so every
// brace-form command after them keeps its adapter (#429). Each source is
// native-valid (zsh -f -n) and must parse into the expected loops and ifs.
func TestHeredocBeforeBraceFormsParses(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		ifs, loops int
	}{
		{"quoted delimiter before if", "cat <<'EOF'\n'\nEOF\nif [[ -n $x ]] { print a }\n", 1, 0},
		{"unquoted delimiter before while", "cat <<EOF\n\"\nEOF\nwhile [[ -n $x ]] { print a }\n", 0, 1},
		{"tab-stripped before for", "cat <<-EOF\n\t'\n\tEOF\nfor x (a b) { print $x }\n", 0, 1},
		{"two documents before multi-name for", "cat <<A <<'B'\n'\nA\n\"\nB\nfor p q (1 2) { print $p }\n", 0, 1},
		{"document after a comment line", "# it's\ncat <<EOF\n'\nEOF\nfor x (a b) { print $x }\n", 0, 1},
		{"here-string before if", "cat <<< \"'\"\nif [[ -n $x ]] { print a }\n", 1, 0},
		{"here-string before for", "cat <<< \"'\"\nfor x (a b) { print $x }\n", 0, 1},
		{"arithmetic shift before if", ": $(( 1 << 2 ))\nif [[ -n $x ]] { print a }\n", 1, 0},
		{"arithmetic command shift before for", "(( y = 3 << 1 ))\nfor x (a b) { print $x }\n", 0, 1},
		{"nested arithmetic parens before if", "(( y = (1 + 2) << (1) ))\nif [[ -n $x ]] { print a }\n", 1, 0},
		{"nested arithmetic parens before for", ": $(( (1) << 2 ))\nfor x (a b) { print $x }\n", 0, 1},
		{"document inside a function", "f() {\ncat <<'EOF'\n'\nEOF\nif [[ -n $x ]] { print a }\n}\n", 1, 0},
		{"document then both forms", "cat <<EOF\n'\nEOF\nfor x (a b) { print $x }\nwhile [[ -n $x ]] { print a }\n", 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parseWithAdapters([]byte(tt.src), "t.zsh")
			if err != nil {
				t.Fatalf("%q: %v", tt.src, err)
			}
			var ifs, loops int
			syntax.Walk(file, func(node syntax.Node) bool {
				switch node.(type) {
				case *syntax.IfClause:
					ifs++
				case *syntax.WhileClause, *syntax.ForClause:
					loops++
				}
				return true
			})
			if ifs != tt.ifs || loops != tt.loops {
				t.Errorf("%q: %d ifs and %d loops, want %d and %d", tt.src, ifs, loops, tt.ifs, tt.loops)
			}
		})
	}
}

// Invalid Zsh after a here-document stays rejected: skipping the body must
// not let an adapter accept what native Zsh refuses (#429).
func TestHeredocQuoteBeforeBraceFormsRejected(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/invalid-429-*.txt")
	if err != nil || len(fixtures) != 3 {
		t.Fatalf("invalid-429 fixtures = %v, %v; want 3", fixtures, err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-429.zsh")
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("Parse() error = %v, want a parse error for a source native Zsh rejects", err)
			}
		})
	}
}

// An unquoted here-document whose text looks like a brace-form construct is
// text: the adapters must not rewrite it while retrying a real construct
// after it (the inverse defect #280 records). The tree's here-document word
// keeps the original lines.
func TestHeredocTextIsNotRewritten(t *testing.T) {
	for _, body := range []string{
		"for x (a b) { print $x }\n",
		"if [[ -n $x ]] { print a }\n",
		"while [[ -n $x ]] { print a }\n",
	} {
		src := "cat <<EOF\n" + body + "EOF\n" + "for y (c d) { print $y }\nif [[ -n $y ]] { print b }\n"
		file, err := Parse(strings.NewReader(src), "t.zsh")
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		var docs []string
		syntax.Walk(file.tree, func(node syntax.Node) bool {
			if redirect, ok := node.(*syntax.Redirect); ok && redirect.Hdoc != nil {
				start, end := redirect.Hdoc.Pos().Offset(), redirect.Hdoc.End().Offset()
				docs = append(docs, src[start:end])
				var printed strings.Builder
				if err := syntax.NewPrinter().Print(&printed, redirect.Hdoc); err != nil {
					t.Fatalf("print here-document: %v", err)
				}
				if printed.String() != body {
					t.Errorf("%q: here-document word prints %q, want %q", src, printed.String(), body)
				}
			}
			return true
		})
		// The word's source span ends after the delimiter line.
		if len(docs) != 1 || docs[0] != body+"EOF" {
			t.Errorf("%q: here-document spans %q, want %q", src, docs, body+"EOF")
		}
	}
}
