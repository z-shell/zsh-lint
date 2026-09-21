package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// zshmisc, Arithmetic Evaluation: an arithmetic expression may call a math
// function, `func(args)`. mvdan/sh has no call node in its arithmetic grammar,
// so the front end reads the call as a parenthesized group and keeps the name
// and arguments as metadata.
func TestMathFunctionCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"one argument", "print $(( sqrt(4) ))\n"},
		{"two arguments", "print $(( min(1,2) ))\n"},
		{"no arguments", "print $(( rand48() ))\n"},
		{"expression argument", "print $(( sqrt(2+2) ))\n"},
		{"nested call", "print $(( sqrt(abs(-4)) ))\n"},
		{"two calls", "print $(( sqrt(4) + min(1,2) ))\n"},
		{"inside a larger expression", "print $(( 1 + sqrt(4) * 2 ))\n"},
		{"arithmetic command", "(( x = sqrt(4) ))\n"},
		{"arithmetic condition", "if (( sqrt(4) > 1 )); then :; fi\n"},
		{"underscore in name", "print $(( my_fn(1) ))\n"},
		{"digit in name", "print $(( rand48(1) ))\n"},
		{"call on both sides", "print $(( min(1,2) + max(3,4) ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The adapter must not rescue what Zsh itself refuses, and must not claim a
// parenthesis that is not a call.
func TestMathFunctionCallRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		// Zsh reports `operator expected`: a call's name touches its `(`.
		{"blank before the parenthesis", "print $(( sqrt (4) ))\n"},
		// A digit-led word is a number, not a function name.
		{"numeric name", "print $(( 4(2) ))\n"},
		{"invalid operator", "print $(( 1 +* 2 ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// A parenthesis outside an arithmetic context opens a subshell, an array
// literal or a glob qualifier. Masking one would change what the source means,
// so these must keep parsing exactly as they did before the adapter existed.
func TestMathFunctionCallLeavesOtherParens(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"subshell", "( print hi )\n"},
		{"array literal", "a=(1 2 3)\n"},
		{"function definition", "f() { print hi }\n"},
		{"command substitution", "print $(echo hi)\n"},
		{"arithmetic grouping", "print $(( (1+2) * 3 ))\n"},
		{"nested arithmetic grouping", "print $(( ((1+2)) ))\n"},
		{"arithmetic for", "for ((i=0;i<2;i++)); do :; done\n"},
		{"parenthesis in a string", "print \"a(b)c\"\n"},
		{"quoted call text", "print 'sqrt(4)'\n"},
		{"case pattern", "case x in y) : ;; esac\n"},
		{"command substitution in arithmetic", "print $(( $(echo 4) + 1 ))\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The mask keeps the call's parentheses, so the group binds exactly where the
// call did. Masking them into a comma expression instead would re-associate
// the expression: `1 + sqrt(4)` would become `(1 + sqrt) , 4`, reading the
// name as a variable and moving the argument out of the call. Assert the
// resulting shape so that regression cannot return unnoticed.
func TestMathFunctionCallKeepsPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"call then operator", "print $(( sqrt(4) + 1 ))\n", "([4] + 1)"},
		{"operator then call", "print $(( 1 + sqrt(4) ))\n", "(1 + [4])"},
		{"multiplication binds tighter", "print $(( sqrt(4) * 2 + 3 ))\n", "(([4] * 2) + 3)"},
		{"arguments stay inside", "print $(( min(1,2) ))\n", "[(1 , 2)]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			var got string
			syntax.Walk(f.AST(), func(n syntax.Node) bool {
				exp, ok := n.(*syntax.ArithmExp)
				if !ok {
					return true
				}
				got = arithmShape(exp.X)
				return false
			})
			if got != tc.want {
				t.Fatalf("shape %s, want %s", got, tc.want)
			}
		})
	}
}

// arithmShape renders an arithmetic expression's structure, so a test can
// assert associativity rather than only that the source parsed.
func arithmShape(expr syntax.ArithmExpr) string {
	switch node := expr.(type) {
	case *syntax.BinaryArithm:
		return "(" + arithmShape(node.X) + " " + node.Op.String() + " " + arithmShape(node.Y) + ")"
	case *syntax.ParenArithm:
		return "[" + arithmShape(node.X) + "]"
	case *syntax.UnaryArithm:
		if node.Post {
			return arithmShape(node.X) + node.Op.String()
		}
		return node.Op.String() + arithmShape(node.X)
	case *syntax.Word:
		return node.Lit()
	}
	return "?"
}

// The name is blanked in the retried source, so it survives only as metadata.
// A fix that parsed the call but lost the name would leave a rule unable to
// tell `sqrt(4)` from the grouping `(4)`.
func TestMathFunctionCallMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		names []string
		args  []int
	}{
		{"one call", "print $(( sqrt(4) ))\n", []string{"sqrt"}, []int{1}},
		{"two arguments", "print $(( min(1,2) ))\n", []string{"min"}, []int{2}},
		{"no arguments", "print $(( rand48() ))\n", []string{"rand48"}, []int{0}},
		{"nested", "print $(( sqrt(abs(-4)) ))\n", []string{"sqrt", "abs"}, []int{1, 1}},
		{"two calls", "print $(( sqrt(4) + min(1,2) ))\n", []string{"sqrt", "min"}, []int{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			calls := f.MathFunctionCalls()
			if len(calls) != len(tc.names) {
				t.Fatalf("%d calls, want %d", len(calls), len(tc.names))
			}
			for i, call := range calls {
				if call.Name.Value != tc.names[i] {
					t.Errorf("call %d named %q, want %q", i, call.Name.Value, tc.names[i])
				}
				if len(call.Args) != tc.args[i] {
					t.Errorf("call %d has %d arguments, want %d", i, len(call.Args), tc.args[i])
				}
				// The name must point at the original source, not the mask.
				want := strings.Index(tc.src, tc.names[i])
				if got := int(call.Name.ValuePos.Offset()); got != want && i == 0 {
					t.Errorf("call %d name at offset %d, want %d", i, got, want)
				}
			}
		})
	}
}

// findMathFunctionCalls is the whole recognition step, so test it directly: a
// scanner that claimed a grouping parenthesis would corrupt the source.
func TestFindMathFunctionCalls(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"single call", "print $(( sqrt(4) ))\n", 1},
		{"two calls", "print $(( sqrt(4) + min(1,2) ))\n", 2},
		{"nested calls", "print $(( sqrt(abs(-4)) ))\n", 2},
		{"arithmetic command", "(( x = sqrt(4) ))\n", 1},
		{"grouping is not a call", "print $(( (1+2) * 3 ))\n", 0},
		{"subshell is not a call", "( print hi )\n", 0},
		{"array literal is not a call", "a=(1 2 3)\n", 0},
		{"definition is not a call", "f() { : }\n", 0},
		{"blank before parenthesis", "print $(( sqrt (4) ))\n", 0},
		{"numeric name", "print $(( 4(2) ))\n", 0},
		{"call text inside quotes", "print 'sqrt(4)'\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(findMathFunctionCalls([]byte(tc.src))); got != tc.want {
				t.Errorf("found %d calls, want %d", got, tc.want)
			}
		})
	}
}

// Sources native Zsh rejects, kept as .txt so the repository-wide `zsh -n`
// gate never sees them.
func TestMathFunctionCallInvalidFixtures(t *testing.T) {
	for _, name := range []string{
		"invalid-233-blank-before-parenthesis.txt",
		"invalid-233-numeric-name.txt",
	} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, err := parseWithAdapters(src, name); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", src)
			}
		})
	}
}
