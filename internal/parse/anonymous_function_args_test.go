package parse

import (
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
