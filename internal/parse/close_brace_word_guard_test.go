package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issues #314 and #316: Zsh's lexer reads an unquoted `}` that ends a word
// as the reserved word, so each fixture is rejected by `zsh -f -n`.
// mvdan/sh reads the byte into the word in a loop's word list, an array
// value, a case word, a test word, a redirect target and after an expansion
// or a quoted part, and the select adapters inherit that; the guard
// positions the parser's own `}` error on the brace, and a select header
// whose list holds a bare one is no site at all.
func TestParseRejectsCloseBraceWords(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-314-for-list.txt", closeBraceWordError, 1, 12},
		{"invalid-314-array-value.txt", closeBraceWordError, 1, 7},
		{"invalid-314-case-word.txt", closeBraceWordError, 1, 6},
		{"invalid-314-paren-for-list.txt", closeBraceWordError, 1, 10},
		{"invalid-314-select-list.txt", "`}` can only be used to close a block", 1, 15},
		{"invalid-314-select-list-braces.txt", "`}` can only be used to close a block", 1, 19},
		{"invalid-314-select-brace-without-term.txt", "`}` can only be used to close a block", 1, 27},
		{"invalid-316-trailing-brace-argument.txt", closeBraceWordError, 1, 8},
		{"invalid-316-trailing-brace-after-expansion.txt", closeBraceWordError, 1, 9},
		{"invalid-316-trailing-brace-after-quoted.txt", closeBraceWordError, 1, 11},
		{"invalid-316-trailing-brace-array-value.txt", closeBraceWordError, 1, 6},
		{"invalid-316-trailing-brace-for-list.txt", closeBraceWordError, 1, 11},
		{"invalid-316-trailing-brace-case-word.txt", closeBraceWordError, 1, 7},
		{"invalid-316-trailing-brace-test-word.txt", closeBraceWordError, 1, 5},
		{"invalid-316-trailing-brace-redirect.txt", closeBraceWordError, 1, 10},
		{"invalid-316-trailing-brace-naked-declaration.txt", closeBraceWordError, 1, 9},
		{"invalid-316-trailing-brace-after-closed-pair.txt", closeBraceWordError, 1, 10},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			assertParseErrorAt(t, src, test.text, test.line, test.col)
		})
	}
}

// The guard sees the final tree, so a `}` an adapter's word list swallowed
// is caught even when the adapter accepted the header: with the select
// scan's own decline removed the bare-word rows would still fail, on the
// `}`. The other rows pin the lexer rule: the brace must end the word, be
// unescaped and unquoted, and close no `{` of the same word; an
// assignment value, a case pattern and a here-document body are exempt.
func TestRejectCloseBraceWordsOnTree(t *testing.T) {
	for _, test := range []struct {
		src  string
		want string // position of the error, or "" when the tree is fine
	}{
		{"for x in a }; do :; done\n", "1:12"},
		{"x=( a } )\n", "1:7"},
		{"case } in (x) ;; esac\n", "1:6"},
		{"print a}\n", "1:8"},
		{"print ${x:-}}\n", "1:13"},
		{"print $(x)}\n", "1:11"},
		{"print \"{\"}\n", "1:10"},
		{"print a\\\\}\n", "1:10"},
		{"print {a}}\n", "1:10"},
		{"print a}}\n", "1:9"},
		{"export a}\n", "1:9"},
		{"[[ a} == b ]]\n", "1:5"},
		{"echo x >a}\n", "1:10"},
		{"for x in '}' \"}\" \\} {; do :; done\n", ""},
		{"cat <<EOF\n}\na}\nEOF\n", ""},
		{"f() { print x }\n", ""},
		{"print a}b {a} a{b} {{a}} a{} a}#c a}${x}\n", ""},
		{"x=a} y=${x}} a}=1\n", ""},
		{"export z=a}\n", ""},
		{"case x in (a}) ;; esac\n", ""},
		{"print a\\}\n", ""},
	} {
		tree, err := parseTree([]byte(test.src), "guard.zsh")
		if err != nil {
			t.Fatalf("parseTree(%q) error: %v", test.src, err)
		}
		err = rejectCloseBraceWords(tree, "guard.zsh")
		if test.want == "" {
			if err != nil {
				t.Errorf("%q: unexpected error %v", test.src, err)
			}
			continue
		}
		var perr syntax.ParseError
		if !errors.As(err, &perr) {
			t.Fatalf("%q: error = %v, want syntax.ParseError", test.src, err)
		}
		if got := perr.Pos.String(); got != test.want || perr.Text != closeBraceWordError {
			t.Errorf("%q: error %s %q, want %s %q", test.src, got, perr.Text, test.want, closeBraceWordError)
		}
	}
}

// Every other spelling of a brace stays a word, as it is for Zsh.
func TestParseKeepsBraceWords(t *testing.T) {
	src, err := os.ReadFile("../survey/testdata/corpus/ok-brace-words.zsh")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if _, err := Parse(bytes.NewReader(src), "ok-brace-words.zsh"); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	for _, row := range []string{"select o in {; break\n", "x=( a { )\n", "print {\n"} {
		if _, err := Parse(strings.NewReader(row), "brace.zsh"); err != nil {
			t.Errorf("Parse(%q) error: %v", row, err)
		}
	}
}
