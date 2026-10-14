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

// A here-document ends at the end of the file when no line closes it, as in
// Zsh (#686), so both sources parse; before the fork read $'...' delimiters
// they were the adapter's genuine unclosed cases. Native: zsh -f -n accepts
// both, and zsh -f prints the rest of the file as the body.
func TestANSICHeredocEndsAtEOF(t *testing.T) {
	for _, tc := range []struct{ src, body string }{
		{"cat <<$'E\\x4fF'\nbody\nMISMATCH\n", "body\nMISMATCH\n"},
		{"cat <<$'EOF'\nbody\n", "body\n"},
		// A decoded newline never matches a line.
		{"cat <<$'E\\nF'\nbody\nE\nF\n", "body\nE\nF\n"},
	} {
		file, err := Parse(strings.NewReader(tc.src), "eof.zsh")
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.src, err)
		}
		stmts := file.AST().Stmts
		if len(stmts) != 1 || len(stmts[0].Redirs) != 1 {
			t.Fatalf("Parse(%q): want one statement with one redirect", tc.src)
		}
		r := stmts[0].Redirs[0]
		if r.ClosePos.IsValid() {
			t.Errorf("Parse(%q): ClosePos = %v, want unset", tc.src, r.ClosePos)
		}
		if got := heredocBody(t, r); got != tc.body {
			t.Errorf("Parse(%q): body = %q, want %q", tc.src, got, tc.body)
		}
	}
}

// The fork decodes the delimiter, so the line holding EOF closes the
// here-document and the next line is a statement of its own; without the
// decoding the body would run to the end of the file (#686, #122).
func TestANSICHeredocDelimiterClosesBody(t *testing.T) {
	const src = "cat <<$'E\\x4fF'\nbody\nEOF\necho after\n"
	file, err := Parse(strings.NewReader(src), "close.zsh")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stmts := file.AST().Stmts
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
	r := stmts[0].Redirs[0]
	if got := heredocBody(t, r); got != "body\n" {
		t.Errorf("body = %q, want %q", got, "body\n")
	}
	if r.ClosePos.Line() != 3 || r.ClosePos.Col() != 1 {
		t.Errorf("ClosePos = %d:%d, want 3:1", r.ClosePos.Line(), r.ClosePos.Col())
	}
	call, ok := stmts[1].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 2 || call.Args[0].Lit() != "echo" || call.Args[1].Lit() != "after" {
		t.Fatalf("second statement = %#v, want echo after", stmts[1].Cmd)
	}
	if stmts[1].Pos().Line() != 4 {
		t.Errorf("second statement at line %d, want 4", stmts[1].Pos().Line())
	}
}

// Control, meta and NUL escapes are not decoded, so such a delimiter stays a
// parse error rather than a stop word that never matches. zsh -f -n accepts
// all but the surrogate, which it reports as a character not in range: the
// others remain known false rejections, as they were with the adapter.
func TestANSICHeredocUnsupportedEscape(t *testing.T) {
	for _, src := range []string{
		"cat <<$'E\\cAF'\nbody\n",
		"cat <<$'E\\C-aF'\nbody\n",
		"cat <<$'E\\M-aF'\nbody\n",
		"cat <<$'E\\x00F'\nbody\n",
		"cat <<$'E\\0F'\nbody\n",
		"cat <<$'E\\400F'\nbody\n",
		"cat <<$'E\\xgF'\nbody\n",
		"cat <<$'E\\uD800F'\nbody\n",
	} {
		_, err := Parse(strings.NewReader(src), "escape.zsh")
		var parseErr syntax.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Parse(%q) error = %v, want a ParseError", src, err)
		}
		if !strings.HasPrefix(parseErr.Text, "unsupported escape in here-document delimiter") ||
			parseErr.Pos.Line() != 1 || parseErr.Pos.Col() != 7 {
			t.Errorf("Parse(%q) error = %v, want an unsupported escape at 1:7", src, err)
		}
	}
}

func heredocBody(t *testing.T, r *syntax.Redirect) string {
	t.Helper()
	if r.Hdoc == nil {
		return ""
	}
	var b bytes.Buffer
	if err := syntax.NewPrinter().Print(&b, r.Hdoc); err != nil {
		t.Fatalf("print body: %v", err)
	}
	return b.String()
}
