package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #511: a word ends inside its glob group at `;`, `&`, or a bare `<`
// or `>`, as Zsh's lexer ends it, so the group's `)` is an error.
func TestGlobGroupWordEndRejected(t *testing.T) {
	for _, tc := range []struct {
		file      string
		text      string
		line, col uint
	}{
		{"invalid-511-glob-group-word-end.txt", "a command can only contain words and redirects; encountered `)`", 4, 12},
		{"invalid-511-condition-group-connective.txt", "`)` matches no `(`: the word ended inside the glob group at 4:10", 4, 15},
	} {
		t.Run(tc.file, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-511.zsh")
			var pe syntax.ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error = %v, want syntax.ParseError", err)
			}
			if pe.Text != tc.text || pe.Pos.Line() != tc.line || pe.Pos.Col() != tc.col {
				t.Fatalf("error = %v, want %d:%d: %s", err, tc.line, tc.col, tc.text)
			}
		})
	}
}
