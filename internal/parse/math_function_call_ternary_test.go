package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A math function call is valid anywhere an operand is (zshmisc, Arithmetic
// Evaluation), including inside a ternary's branches. The retry is gated on
// the parser's first error, and a call after a `?` produces a different one:
// the parser takes the name as the true-branch operand and then requires the
// `:`, reporting the unfinished ternary rather than the `(`.
func TestMathFunctionCallInTernary(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"true branch", "print $(( 1 ? sqrt(4) : 3 ))\n"},
		{"both branches", "print $(( 1 ? sqrt(4) : sqrt(9) ))\n"},
		{"true branch with no arguments", "print $(( 1 ? rand48() : 3 ))\n"},
		{"true branch with two arguments", "print $(( 1 ? fmod(7,4) : 3 ))\n"},
		{"true branch in a larger expression", "print $(( 1 ? sqrt(4)+1 : 3 ))\n"},
		{"nested ternary", "print $(( 1 ? (2 ? sqrt(4) : 3) : 5 ))\n"},
		{"arithmetic command", "(( x = 1 ? sqrt(4) : 3 ))\n"},
		{"call in the condition and the true branch", "print $(( sqrt(4) ? sqrt(4) : 3 ))\n"},
		{"false branch only", "print $(( 1 ? 2 : sqrt(9) ))\n"},
		{"condition only", "print $(( sqrt(4) ? 2 : 3 ))\n"},
		{"nested call in the true branch", "print $(( 1 ? int(sqrt(16)) : 3 ))\n"},

		// A call is an operand, so it stands anywhere an operand may. Each of
		// these reports the same ternary error, so a gate that looked only at
		// the operand position immediately after the `?` would miss them.
		//
		// A call inside a parenthesized group, `1 ? (sqrt(4)) : 3`, is NOT
		// here: it reports a third error, ``reached `(` without matching `(`
		// with `)` ``, which this adapter does not own. That shape fails
		// without a ternary too (`$(( (sqrt(4)) ))`), so it is a separate gap.
		{"call after a unary minus", "print $(( 1 ? -sqrt(4) : 3 ))\n"},
		{"call as a right operand", "print $(( 1 ? 2*sqrt(4) : 3 ))\n"},
		{"call after a binary operator and a blank", "print $(( 1 ? 2 + sqrt(4) : 3 ))\n"},

		// Arithmetic is not line-oriented, so a ternary may be written across
		// lines. This is the shape the gap was found in: `z-shell/zi`
		// `zi.zsh:2749` breaks immediately after the `?`.
		{"call on the line after the `?`", "print $(( 1 ?\n  sqrt(4) : 3 ))\n"},
		{"branches on their own lines", "print $(( 1 ?\nsqrt(4)\n: 3 ))\n"},
		{"blank line between `?` and the call", "print $(( 1 ?\n\n  sqrt(4) : 3 ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The wider gate must not rescue a malformed ternary. `zsh -n` accepts each of
// these, because it does not evaluate arithmetic, but `zsh -f` fails them at
// runtime (`bad math expression: ':' expected`), so rejecting them is correct
// and they are not gaps. They hold no call site, so the retry finds nothing to
// mask and the parser error stands.
func TestMathFunctionCallTernaryRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"missing false branch", "print $(( 1 ? 2 ))\n"},
		{"missing both branches", "print $(( 1 ? ))\n"},
		{"missing true branch", "print $(( 1 ? : 3 ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// A malformed ternary keeps its own error even when the file holds calls
// elsewhere. The gate is bounded to the reported `?`'s own arithmetic
// expression, so an unrelated call cannot make a broken ternary retry.
func TestMathFunctionCallTernaryRejectsBesideUnrelatedCalls(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"call in an earlier expression", "print $(( sqrt(4) ))\nprint $(( 1 ? 2 ))\n"},
		{"call in a later expression", "print $(( 1 ? 2 ))\nprint $(( sqrt(4) ))\n"},
		{"calls on both sides", "print $(( sqrt(4) ))\nprint $(( 1 ? 2 ))\nprint $(( sqrt(9) ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// Every adapter is gated on its own concrete parser error, so a file failing
// for an unrelated reason is left to the next adapter and reports the error it
// really had. Without this the retry would mask call sites on any failure and
// could report a misleading error for an unrelated defect.
func TestMathFunctionCallGate(t *testing.T) {
	// A source that does hold a call site, so only the gate can stop the retry.
	src := []byte("print $(( sqrt(4) ))\n")
	unrelated := syntax.ParseError{Text: "some unrelated error"}

	got, err := parseMathFunctionCall(src, "t.zsh", unrelated)
	if got != nil {
		t.Fatal("adapter retried on an unrelated error")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != unrelated.Text {
		t.Fatalf("returned %v, want the incoming error unchanged", err)
	}

	// The operator error names the call's own `(`, so it is accepted on any
	// source that holds a site.
	if _, err := parseMathFunctionCall(src, "t.zsh", syntax.ParseError{Text: invalidMathFunctionCall}); err != nil {
		t.Errorf("adapter refused its own error %q: %v", invalidMathFunctionCall, err)
	}

	// The ternary error names the `?`, not a call, so it is accepted only
	// where a call really stands in that `?`'s expression. Take the position
	// from the upstream parser rather than inventing one: the adapter chain
	// now fixes this source, so the raw parser is the only place the error
	// still comes from.
	ternary := []byte("print $(( 1 ? sqrt(4) : 3 ))\n")
	if _, err := parseMathFunctionCall(ternary, "t.zsh", rawTernaryError(t, ternary)); err != nil {
		t.Errorf("adapter refused its own error %q: %v", incompleteTernaryMathCall, err)
	}

	// The same error on a source whose `?` expression has no call is left
	// alone, even though another expression in the file does hold one.
	plain := []byte("print $(( 1 ? 2 ))\nprint $(( sqrt(4) ))\n")
	plainErr := rawTernaryError(t, []byte("print $(( 1 ? 2 ))\n"))
	if _, err := parseMathFunctionCall(plain, "t.zsh", plainErr); err == nil {
		t.Error("adapter retried a ternary error whose expression holds no call")
	}
}

// The adapter is entered only for a real parser error. A non-parse error must
// pass straight through: the chain distinguishes adapters by the concrete
// error each one owns, and treating an unknown failure as this adapter's would
// mask a source for a defect that has nothing to do with a call.
func TestMathFunctionCallRequiresAParseError(t *testing.T) {
	src := []byte("print $(( sqrt(4) ))\n")
	plain := errors.New("not a parse error")

	got, err := parseMathFunctionCall(src, "t.zsh", plain)
	if got != nil {
		t.Fatal("adapter retried on a non-parse error")
	}
	if !errors.Is(err, plain) {
		t.Fatalf("returned %v, want the incoming error unchanged", err)
	}
}

// The gate is the arithmetic expression that holds the reported `?`, not the
// whole file and not one operand position. Assert that boundary directly: the
// end-to-end tests above cannot distinguish "found the call in this
// expression" from "found a call anywhere".
func TestTernaryBranchHoldsCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want bool
	}{
		{"call immediately after the `?`", "print $(( 1 ? f(2) : 3 ))\n", true},
		{"call further into the branch", "print $(( 1 ? 2 + f(3) : 4 ))\n", true},
		{"call in the false branch", "print $(( 1 ? 2 : f(3) ))\n", true},
		{"call on a later line of the same expression", "print $(( 1 ?\nf(2) : 3 ))\n", true},

		{"no call at all", "print $(( 1 ? 2 ))\n", false},
		{"call only before the `?`", "print $(( f(1) ? 2 : 3 ))\n", false},
		{"call only in a later expression", "print $(( 1 ? 2 ))\nprint $(( f(3) ))\n", false},
		{"call only in an earlier expression", "print $(( f(3) ))\nprint $(( 1 ? 2 ))\n", false},

		// The span looked up must be the one holding the reported `?`, not
		// simply the file's first. Here the ternary is in the second
		// expression, so taking the first would wrongly report no call.
		{"ternary in the second expression", "print $(( 1 ))\nprint $(( 1 ? f(2) : 3 ))\n", true},
		{"ternary in the third expression", "print $(( 1 ))\nprint $(( 2 ))\nprint $(( 1 ? f(2) : 3 ))\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			// The reported `?` is the one the parser stops at, which is the
			// first one in each of these sources.
			offset := bytes.IndexByte(src, '?')
			if offset < 0 {
				t.Fatal("test source holds no `?`")
			}
			if got := ternaryBranchHoldsCall(src, findMathFunctionCalls(src), offset); got != tc.want {
				t.Errorf("ternaryBranchHoldsCall = %v, want %v", got, tc.want)
			}
		})
	}

	// The offset must actually name a `?`. The anonymous-invocation retry
	// hands this adapter errors from rewritten intermediate sources, where an
	// offset can land anywhere; treating a non-`?` position as a ternary would
	// retry a source the parser never complained about for this reason.
	//
	// Pick a position that a call really does follow inside the same
	// expression, so only the `?` check itself can reject it.
	src := []byte("print $(( 1 ? f(2) : 3 ))\n")
	sites := findMathFunctionCalls(src)
	notQuestion := bytes.IndexByte(src, '1')
	if src[notQuestion] == '?' {
		t.Fatalf("test setup: offset %d is a `?`", notQuestion)
	}
	if ternaryBranchHoldsCall(src, sites, notQuestion) {
		t.Error("accepted an offset that is not a `?`")
	}
	for _, offset := range []int{-1, len(src), len(src) + 100} {
		if ternaryBranchHoldsCall(src, sites, offset) {
			t.Errorf("accepted out-of-range offset %d", offset)
		}
	}
}

// The retry recovers every call in the file, not only the one the reported `?`
// names. Masking is per file by design, so a second call elsewhere must come
// back with its own name and original offsets.
func TestMathFunctionCallTernaryRecoversEveryCall(t *testing.T) {
	src := []byte("print $(( 1 ? g(2) : 3 ))\nprint $(( h(4) ))\n")

	tree, err := parseMathFunctionCall(src, "t.zsh", rawTernaryError(t, src))
	if err != nil {
		t.Fatalf("adapter refused its own error: %v", err)
	}
	calls := bindMathFunctionCalls(tree, src)
	if len(calls) != 2 {
		t.Fatalf("recovered %d calls, want 2", len(calls))
	}
	for i, want := range []string{"g", "h"} {
		if got := calls[i].Name.Value; got != want {
			t.Errorf("call %d recovered %q, want %q", i, got, want)
		}
	}
}

// Accepting the call is not enough: the ternary must survive it. A mask that
// consumed the `:` would still parse as something, so assert the structure and
// the retained call metadata.
func TestMathFunctionCallTernaryShape(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		want  string
		names []string
	}{
		{"true branch", "print $(( 1 ? sqrt(4) : 3 ))\n", "(1 ? ([4] : 3))", []string{"sqrt"}},
		{"both branches", "print $(( 1 ? sqrt(4) : sqrt(9) ))\n", "(1 ? ([4] : [9]))", []string{"sqrt", "sqrt"}},
		{"no arguments", "print $(( 1 ? rand48() : 3 ))\n", "(1 ? (0 : 3))", []string{"rand48"}},
		{"two arguments", "print $(( 1 ? fmod(7,4) : 3 ))\n", "(1 ? ([(7 , 4)] : 3))", []string{"fmod"}},
		// The multi-line form must build the same tree as the one-line form:
		// a newline is a separator inside arithmetic, not a terminator.
		{"call on the next line", "print $(( 1 ?\nsqrt(4) : 3 ))\n", "(1 ? ([4] : 3))", []string{"sqrt"}},
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
				// The name must point at the original source, not the mask.
				offset := int(call.Name.ValuePos.Offset())
				if offset < 0 || offset+len(call.Name.Value) > len(tc.src) ||
					tc.src[offset:offset+len(call.Name.Value)] != call.Name.Value {
					t.Errorf("call %d name offset %d does not hold %q", i, offset, call.Name.Value)
				}
			}
		})
	}
}

// rawTernaryError returns the ternary error the upstream parser reports for
// src, so a test gates the adapter on a real position rather than an invented
// one.
func rawTernaryError(t *testing.T, src []byte) syntax.ParseError {
	t.Helper()
	_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != incompleteTernaryMathCall {
		t.Fatalf("want the ternary error from the parser, got %v", err)
	}
	return parseErr
}
