package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNativeANSICHeredocCompatibility(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "hex escape delimiter",
			src:  "cat <<$'E\\x4fF'\nbody\nEOF\n",
			want: "$'E\\x4fF'",
		},
		{
			name: "octal escape delimiter",
			src:  "cat <<$'E\\117F'\nbody\nEOF\n",
			want: "$'E\\117F'",
		},
		{
			name: "unicode escape delimiter",
			src:  "cat <<$'E\\u004fF'\nbody\nEOF\n",
			want: "$'E\\u004fF'",
		},
		{
			name: "strip tabs heredoc with spaces",
			src:  "cat <<-  $'E\\x4fF'\n\tbody\n\tEOF\n",
			want: "$'E\\x4fF'",
		},
		{
			name: "heredoc with trailing command redirection",
			src:  "cat <<$'E\\x4fF' >out.txt\nbody\nEOF\n",
			want: "$'E\\x4fF'",
		},
		{
			name: "multiple ANSI-C heredocs",
			src:  "cat <<$'H\\x45AD'\nheader\nHEAD\ncat <<$'T\\x41IL'\ntail\nTAIL\n",
			want: "$'H\\x45AD'",
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
				t.Errorf("printed AST = %q, want original delimiter %q", rendered.String(), test.want)
			}
		})
	}
}

func TestANSICHeredocPreservesLaterErrorPosition(t *testing.T) {
	const src = "cat <<$'E\\x4fF'\nbody\nEOF\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted a trailing unmatched parenthesis")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 4 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 4:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

func TestANSICHeredocRejectsGenuineUnclosedHeredoc(t *testing.T) {
	for _, src := range []string{
		"cat <<$'E\\x4fF'\nbody\nMISMATCH\n",
		"cat <<$'EOF'\nbody\n",
	} {
		if _, err := Parse(strings.NewReader(src), "unclosed.zsh"); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}
