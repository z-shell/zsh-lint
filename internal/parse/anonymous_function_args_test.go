package parse

import (
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestAnonymousFunctionInvocationArguments(t *testing.T) {
	source := `() {
  builtin emulate -L zsh
  typeset -r source_path=$1
} "${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}" second
`
	file, err := Parse(strings.NewReader(source), "entrypoint.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	invocations := file.AnonymousInvocations()
	if len(invocations) != 1 {
		t.Fatalf("invocation count = %d, want 1", len(invocations))
	}
	if invocations[0].Function == nil || invocations[0].Function.Name != nil {
		t.Fatal("invocation is not paired with its anonymous function")
	}
	if len(invocations[0].Words) != 2 {
		t.Fatalf("word count = %d, want 2", len(invocations[0].Words))
	}
	if got := getParseWordLiteral(invocations[0].Words[1]); got != "second" {
		t.Fatalf("second word = %q, want second", got)
	}
	wantOffset := strings.Index(source, `"${ZERO`)
	if got := int(invocations[0].Words[0].Pos().Offset()); got != wantOffset {
		t.Fatalf("first word offset = %d, want %d", got, wantOffset)
	}

	parameters := map[string]bool{}
	syntax.Walk(invocations[0].Words[0], func(node syntax.Node) bool {
		if expansion, ok := node.(*syntax.ParamExp); ok && expansion.Param != nil {
			parameters[expansion.Param.Value] = true
		}
		return true
	})
	for _, name := range []string{"ZERO", "0", "ZSH_ARGZERO"} {
		if !parameters[name] {
			t.Errorf("first word did not retain parameter %q", name)
		}
	}
}

func TestNestedAnonymousFunctionInvocationArguments(t *testing.T) {
	source := `() {
  () {
    print -r -- "$1"
  } inner
} outer
`
	file, err := Parse(strings.NewReader(source), "nested.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	invocations := file.AnonymousInvocations()
	if len(invocations) != 2 {
		t.Fatalf("invocation count = %d, want 2", len(invocations))
	}
	if got := getParseWordLiteral(invocations[0].Words[0]); got != "inner" {
		t.Fatalf("first invocation word = %q, want inner", got)
	}
	if got := getParseWordLiteral(invocations[1].Words[0]); got != "outer" {
		t.Fatalf("second invocation word = %q, want outer", got)
	}
}

func TestRepeatedAnonymousFunctionInvocationArguments(t *testing.T) {
	source := "() { :; } one\n() { :; } two\n"
	file, err := Parse(strings.NewReader(source), "repeated.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := len(file.AnonymousInvocations()); got != 2 {
		t.Fatalf("invocation count = %d, want 2", got)
	}
}

// TestAnonymousFunctionInvocationAcrossLineContinuation regression-tests a
// `\`-newline between the closing `}` and its invocation word: the parse
// error's position lands on the continuation line, which has no `}` of its
// own, so the candidate search must walk back across the continuation to
// find the one on the previous line.
func TestAnonymousFunctionInvocationAcrossLineContinuation(t *testing.T) {
	source := "() { x; } \\\narg\n"
	file, err := Parse(strings.NewReader(source), "continuation.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	invocations := file.AnonymousInvocations()
	if len(invocations) != 1 {
		t.Fatalf("invocation count = %d, want 1", len(invocations))
	}
	if got := getParseWordLiteral(invocations[0].Words[0]); got != "arg" {
		t.Fatalf("invocation word = %q, want arg", got)
	}
}

func TestAnonymousFunctionArgumentsComposeWithEarlierAdapters(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "alternate if",
			source: `() {
  if [[ -n $value ]] {
    print -r -- "$value"
  }
} argument
`,
		},
		{
			name: "associative subscript",
			source: `() {
  (( ${+functions[.handler]} ))
} argument
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.source), "composed.zsh")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			invocations := file.AnonymousInvocations()
			if len(invocations) != 1 {
				t.Fatalf("invocation count = %d, want 1", len(invocations))
			}
			if got := getParseWordLiteral(invocations[0].Words[0]); got != "argument" {
				t.Fatalf("invocation word = %q, want argument", got)
			}
		})
	}
}

func TestAnonymousFunctionInvocationControls(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "named function", source: "named() { :; } argument\n"},
		{name: "ordinary block", source: "{ :; } argument\n"},
		{name: "unterminated word", source: "() { :; } \"unterminated\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, originalErr := parseTree([]byte(test.source), "invalid.zsh")
			if originalErr == nil {
				t.Fatal("control source did not reproduce an upstream parse error")
			}
			_, err := Parse(strings.NewReader(test.source), "invalid.zsh")
			if err == nil {
				t.Fatal("Parse() accepted control source")
			}
			if err.Error() != originalErr.Error() {
				t.Fatalf("Parse() error = %q, want original %q", err, originalErr)
			}
		})
	}
}

// TestAnonymousFunctionArgsRetryReportsLaterBlocker regression-tests part of
// issue #255: once a candidate anonymous invocation is masked and resolved,
// a genuinely separate, later syntax error in the rest of the file must be
// reported at its own position, not as the stale error the first, unmasked
// parse attempt produced against the resolved candidate.
func TestAnonymousFunctionArgsRetryReportsLaterBlocker(t *testing.T) {
	source := "f() {\n  () { x; } y\n}\n)\n"
	_, err := Parse(strings.NewReader(source), "later-blocker.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted a trailing unmatched `)`")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error type = %T, want syntax.ParseError", err)
	}
	if parseErr.Pos.Line() != 4 || parseErr.Pos.Col() != 1 {
		t.Fatalf("error position = %d:%d, want 4:1 (the real later blocker, not the resolved candidate)", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

func TestAnonymousFunctionTextControlsRemainOrdinary(t *testing.T) {
	source := `print -r -- '} argument'
# } argument
cat <<'BODY'
} argument
BODY
`
	file, err := Parse(strings.NewReader(source), "text-controls.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := len(file.AnonymousInvocations()); got != 0 {
		t.Fatalf("invocation count = %d, want 0", got)
	}
}
