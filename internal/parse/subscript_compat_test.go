package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNativeAssociativeSubscriptKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "punctuated assignment key",
			src:  "ZI[annex-before-load:new-@]=value\n",
			want: "annex-before-load:new-@",
		},
		{
			name: "leading dot expansion key",
			src:  "print -r -- ${functions[.foo]}\n",
			want: ".foo",
		},
		{
			name: "leading at expansion key",
			src:  "print -r -- ${functions[@zi-register-annex]}\n",
			want: "@zi-register-annex",
		},
		{
			name: "leading at expansion key foo",
			src:  "print -r -- ${functions[@foo]}\n",
			want: "@foo",
		},
		{
			name: "leading at hash expansion key",
			src:  "print -r -- ${myhash[@zi]}\n",
			want: "@zi",
		},
		{
			name: "annex guard parameter expansion",
			src:  "(( ${+functions[@zi-register-annex]} ))\n",
			want: "@zi-register-annex",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.want) {
				t.Errorf("printed AST = %q, want original subscript key %q", rendered.String(), test.want)
			}
		})
	}
}

func TestAssociativeSubscriptCompatibilityPreservesLaterErrorPosition(t *testing.T) {
	const src = "print -r -- ${functions[.foo]}\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted a trailing unmatched parenthesis")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 2:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

func TestAssociativeSubscriptCompatibilityRejectsMalformedKeys(t *testing.T) {
	for _, src := range []string{
		"print -r -- ${functions[.foo}\n",
		"ZI[annex-before-load:new-@=value\n",
	} {
		if _, err := Parse(strings.NewReader(src), "malformed.zsh"); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}
