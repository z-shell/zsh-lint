package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #314: Zsh never lexes an unquoted `}` as a word, so each fixture is
// rejected by `zsh -f -n`. mvdan/sh reads the byte as a word in a loop's
// word list, an array value and a case word, and the select adapters
// inherit that; the guard positions the parser's own `}` error on the word,
// and a select header whose list holds one is no site at all.
func TestParseRejectsBareCloseBraceWords(t *testing.T) {
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
		{"invalid-314-select-list.txt", "`select foo [in words]` must be followed by `do`", 1, 1},
		{"invalid-314-select-list-braces.txt", "`select foo [in words]` must be followed by `do`", 1, 1},
		{"invalid-314-select-brace-without-term.txt", "`select foo [in words]` must be followed by `do`", 1, 1},
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
// scan's own decline removed this row would still fail, on the `}`.
func TestRejectBareCloseBraceWordsOnTree(t *testing.T) {
	for _, test := range []struct {
		src  string
		want string // position of the error, or "" when the tree is fine
	}{
		{"for x in a }; do :; done\n", "1:12"},
		{"x=( a } )\n", "1:7"},
		{"case } in (x) ;; esac\n", "1:6"},
		{"for x in '}' \"}\" \\} {; do :; done\n", ""},
		{"cat <<EOF\n}\nEOF\n", ""},
		{"print ${x:-}}\n", ""},
		{"f() { print x }\n", ""},
	} {
		tree, err := parseTree([]byte(test.src), "guard.zsh")
		if err != nil {
			t.Fatalf("parseTree(%q) error: %v", test.src, err)
		}
		err = rejectBareCloseBraceWords(tree, "guard.zsh")
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

// A select list that holds a bare `}` reports no site, so the adapter does
// not probe or retry for it.
func TestScanSelectWordListStopsAtBareCloseBrace(t *testing.T) {
	for _, src := range []string{"select o in a }; break\n", "select o in a b c { break }\n"} {
		if term, ok := scanSelectWordList([]byte(src), len("select o in")); ok {
			t.Errorf("scanSelectWordList(%q) = %d, true; want no site", src, term)
		}
	}
	if _, ok := scanSelectWordList([]byte("select o in '}' {; break\n"), len("select o in")); !ok {
		t.Error("a quoted `}` and a bare `{` are words; want a site")
	}
}
