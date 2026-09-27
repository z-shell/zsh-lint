package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A math function call directly inside a parenthesized group makes the parser
// report the group as unmatched at the group's own `(` (#356). Each row is
// valid under `zsh -f -n` on Zsh 5.9.2.
func TestMathFunctionCallInGroup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		want  string
		names []string
	}{
		{"bare", "print $(( (sqrt(4)) ))\n", "[[4]]", []string{"sqrt"}},
		{"nested groups", "print $(( ((sqrt(4))) ))\n", "[[[4]]]", []string{"sqrt"}},
		{"right operand", "print $(( 1 + (sqrt(4)) ))\n", "(1 + [[4]])", []string{"sqrt"}},
		{"ternary branch", "print $(( 1 ? (sqrt(4)) : 3 ))\n", "(1 ? ([[4]] : 3))", []string{"sqrt"}},
		{"group then product", "print $(( (sqrt(4) + 1) * 2 ))\n", "([([4] + 1)] * 2)", []string{"sqrt"}},
		{"call after an operand", "print $(( (1 + sqrt(4)) ))\n", "[(1 + [4])]", []string{"sqrt"}},
		{"two calls", "print $(( 2 * (sqrt(16) - sqrt(4)) ))\n", "(2 * [([16] - [4])])", []string{"sqrt", "sqrt"}},
		{"no arguments", "print $(( (rand48()) < 2 ))\n", "([0] < 2)", []string{"rand48"}},
		{"two arguments", "print $(( (max(1,2)) ))\n", "[[(1 , 2)]]", []string{"max"}},
		{"unary minus", "print $(( (-sqrt(4)) ))\n", "[-[4]]", []string{"sqrt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			var got string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				exp, ok := node.(*syntax.ArithmExp)
				if !ok {
					return true
				}
				got = arithmShape(exp.X)
				return false
			})
			if got != tc.want {
				t.Errorf("shape %s, want %s", got, tc.want)
			}
			calls := file.MathFunctionCalls()
			if len(calls) != len(tc.names) {
				t.Fatalf("%d calls, want %d", len(calls), len(tc.names))
			}
			for i, call := range calls {
				if call.Name.Value != tc.names[i] {
					t.Errorf("call %d named %q, want %q", i, call.Name.Value, tc.names[i])
				}
				offset := int(call.Name.ValuePos.Offset())
				if tc.src[offset:offset+len(call.Name.Value)] != call.Name.Value {
					t.Errorf("call %d name offset %d does not hold %q", i, offset, call.Name.Value)
				}
			}
		})
	}

	// The arithmetic command and a quoted expansion take the same path.
	for _, src := range []string{
		"(( (sqrt(4)) > 1 )) && print y\n",
		"x=$(( (sqrt(4)) ))\n",
		"print \"$(( (sqrt(4)) ))\"\n",
		"print $(( (sqrt(4)) )) $(( (1 + 2) ))\n",
	} {
		if _, err := parseWithAdapters([]byte(src), "t.zsh"); err != nil {
			t.Errorf("valid Zsh rejected: %q: %v", src, err)
		}
	}
}

// An unbalanced group keeps its own error. Each row is rejected by
// `zsh -f -n`, and the retry must not rescue it even though it holds a call.
func TestMathFunctionCallInGroupRejects(t *testing.T) {
	for _, src := range []string{
		"print $(( (sqrt(4) ))\n",
		"print $(( (sqrt(4) ) ) ))\n",
		"print $(( (1 + 2 ))\n",
	} {
		if _, err := parseWithAdapters([]byte(src), "t.zsh"); err == nil {
			t.Errorf("invalid Zsh accepted: %q", src)
		}
	}
}

// The gate is the reported group itself: a call must stand wholly inside the
// group's own parentheses, in the same arithmetic expression. `group` names
// the reported `(` by the text that starts there.
func TestGroupHoldsCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		group string
		want  bool
	}{
		{"call is the group's operand", "print $(( (f(2)) ))\n", "(f(2))", true},
		{"call later in the group", "print $(( (1 + f(2)) ))\n", "(1 + f", true},
		{"call in a nested group", "print $(( ((f(2))) ))\n", "((f(2)))", true},

		{"group with no call", "print $(( (1 + 2) ))\n", "(1 + 2)", false},
		{"call after the group", "print $(( (1 + 2) + f(3) ))\n", "(1 + 2)", false},
		{"call before the group", "print $(( f(3) + (1 + 2) ))\n", "(1 + 2)", false},
		{"call in another expression", "print $(( (1 + 2) ))\nprint $(( f(3) ))\n", "(1 + 2)", false},
		{"call in an earlier expression", "print $(( f(3) ))\nprint $(( (1 + 2) ))\n", "(1 + 2)", false},
		// A `(` outside any arithmetic expression is never this adapter's,
		// even when a call stands between it and its `)`.
		{"subshell around the expression", "( print $(( f(2) )) )\n", "( print", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			offset := bytes.Index(src, []byte(tc.group))
			if offset < 0 || src[offset] != '(' {
				t.Fatalf("test setup: %q does not start at a `(`", tc.group)
			}
			if got := groupHoldsCall(src, findMathFunctionCalls(src), offset); got != tc.want {
				t.Errorf("groupHoldsCall = %v, want %v", got, tc.want)
			}
		})
	}

	// The offset must name a `(`. The blank before a group that does hold a
	// call is inside the same expression and before the call, so only the
	// `(` check can reject it.
	src := []byte("print $(( (f(2)) ))\n")
	sites := findMathFunctionCalls(src)
	blank := bytes.Index(src, []byte(" (f"))
	if groupHoldsCall(src, sites, blank) {
		t.Error("accepted an offset that is not a `(`")
	}
	for _, offset := range []int{-1, len(src), len(src) + 100} {
		if groupHoldsCall(src, sites, offset) {
			t.Errorf("accepted out-of-range offset %d", offset)
		}
	}
}

// The adapter owns the unmatched-group error only where a call stands in the
// reported group. The error must come from the upstream parser, so the
// position is real.
func TestMathFunctionCallGroupGate(t *testing.T) {
	src := []byte("print $(( (sqrt(4)) ))\n")
	if _, err := parseMathFunctionCall(src, "t.zsh", rawGroupError(t, src)); err != nil {
		t.Errorf("adapter refused its own error %q: %v", unmatchedGroupMathCall, err)
	}

	// The same error text on a group holding no call is left alone, even
	// though another expression in the file does hold one.
	plain := []byte("print $(( (a b) ))\nprint $(( sqrt(4) ))\n")
	plainErr := syntax.ParseError{Text: unmatchedGroupMathCall, Pos: syntax.NewPos(uint(bytes.Index(plain, []byte("(a"))), 1, 11)}
	got, err := parseMathFunctionCall(plain, "t.zsh", plainErr)
	if got != nil {
		t.Error("adapter retried an unmatched group that holds no call")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != unmatchedGroupMathCall {
		t.Errorf("returned %v, want the incoming error unchanged", err)
	}
}

// rawGroupError returns the unmatched-group error the upstream parser reports
// for src.
func rawGroupError(t *testing.T, src []byte) syntax.ParseError {
	t.Helper()
	_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != unmatchedGroupMathCall {
		t.Fatalf("want the unmatched-group error from the parser, got %v", err)
	}
	return parseErr
}
