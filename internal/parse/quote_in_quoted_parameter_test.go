package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// In a double-quoted `${...}` word a `'` is text (#400), read by the parser
// fork's lexer. The expansion's word and the literal after it must have the
// extent `zsh -f` gives them: "${x:-'a}'b}" prints 'a'b}, so the default word
// is 'a and the first `}` closes the expansion.
func TestQuoteInQuotedParameterWordExtent(t *testing.T) {
	for _, tt := range []struct {
		src, word, rest string
	}{
		{`print -r -- "${x:-'a}'b}"`, `'a`, `'b}`},
		{`print -r -- "${x:-it's}"`, `it's`, ``},
		{`print -r -- "${x#'}"`, `'`, ``},
		{`print -r -- "${x:-'}x"`, `'`, `x`},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Fatalf("%s: %v", tt.src, err)
		}
		var quoted *syntax.DblQuoted
		syntax.Walk(file.tree, func(node syntax.Node) bool {
			if dq, ok := node.(*syntax.DblQuoted); ok && quoted == nil {
				quoted = dq
				return false
			}
			return true
		})
		if quoted == nil || len(quoted.Parts) == 0 {
			t.Fatalf("%s: no double-quoted word", tt.src)
		}
		pe, ok := quoted.Parts[0].(*syntax.ParamExp)
		if !ok || pe.Exp == nil || pe.Exp.Word == nil {
			t.Fatalf("%s: first part %T, want a parameter expansion with a word", tt.src, quoted.Parts[0])
		}
		if got := pe.Exp.Word.Lit(); got != tt.word {
			t.Errorf("%s: expansion word %q, want %q", tt.src, got, tt.word)
		}
		var rest strings.Builder
		for _, part := range quoted.Parts[1:] {
			if lit, ok := part.(*syntax.Lit); ok {
				rest.WriteString(lit.Value)
			}
		}
		if rest.String() != tt.rest {
			t.Errorf("%s: text after the expansion %q, want %q", tt.src, rest.String(), tt.rest)
		}
	}
}

// Outside double quotes the quote still opens a single-quoted string.
func TestQuoteInUnquotedParameterWordStaysQuoting(t *testing.T) {
	file, err := Parse(strings.NewReader("print -r -- ${x:-'a b'}\n"), "t.zsh")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	syntax.Walk(file.tree, func(node syntax.Node) bool {
		if sq, ok := node.(*syntax.SglQuoted); ok && sq.Value == "a b" {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("unquoted ${x:-'a b'} lost its single-quoted string")
	}
}

func TestQuoteInQuotedParameterWordRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-400-unterminated-string.txt")
	if err != nil {
		t.Fatal(err)
	}
	var parseErr syntax.ParseError
	if _, err := Parse(bytes.NewReader(src), "invalid-400.zsh"); !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
}
