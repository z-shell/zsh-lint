package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNativeFdVarRedirectCompatibility(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "fd var close redirect",
			src:  "exec {fd}>&-\n",
			want: "{fd}>&-",
		},
		{
			name: "fd var input close redirect",
			src:  "exec {fd}<&-\n",
			want: "{fd}<&-",
		},
		{
			name: "fd var open output redirect",
			src:  "exec {fd}>file\n",
			want: "{fd}>file",
		},
		{
			name: "fd var open input redirect",
			src:  "exec {fd}<file\n",
			want: "{fd}<file",
		},
		{
			name: "fd var duplicate redirect",
			src:  "exec {fd}>&1\n",
			want: "{fd}>&1",
		},
		{
			name: "multiple fd var redirects",
			src:  "exec {in}<input.txt\nexec {out}>output.txt\nexec {out}>&-\nexec {in}<&-\n",
			want: "{in}<input.txt",
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
				t.Errorf("printed AST = %q, want original redirect %q", rendered.String(), test.want)
			}
		})
	}
}

func TestFdVarRedirectPreservesLaterErrorPosition(t *testing.T) {
	for _, src := range []string{
		"exec {fd}>&-\n)\n",
		"exec {fd}>file\n)\n",
	} {
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
}

func TestFdVarRedirectRejectsMalformedForms(t *testing.T) {
	for _, src := range []string{
		"exec {fd}>\n",
		"exec {fd}>&\n",
		"exec {fd}<\n",
	} {
		if _, err := Parse(strings.NewReader(src), "malformed.zsh"); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}
