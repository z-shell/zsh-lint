package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #368: a flagged subscript pattern that holds a bracket expression,
// standing in arithmetic. mvdan/sh ends the raw pattern at the bracket
// expression's own `]`, as it does in a parameter expansion (#283), but here
// the rest of the subscript is read as arithmetic. The error text therefore
// names whatever byte follows the cut: `#` is "not a valid arithmetic
// operator", `*` is an operator missing its right operand, `]` is a second
// closer, `.` runs on to a stray `)`.
//
// Every row is `zsh -f -n` valid and evaluates without error under `zsh -f`
// (the corpus fixture ok-flag-pattern-arithmetic.zsh runs the same shapes and
// prints their values). The pattern must come back whole at its original
// offsets.
func TestParseFlagPatternInArithmetic(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
	}{
		{"issue row, `##` after the bracket", "print $(( m[(r)a[^:]##] ))\n", "r", "a[^:]##"},
		{"issue row, nested in an expansion", "print ${$(( m[(r)a[^:]##] ))}\n", "r", "a[^:]##"},
		{"arithmetic command", "(( m[(i)a[^:]##] ))\n", "i", "a[^:]##"},
		{"bracket expression last", "print $(( m[(i)a[bc]] ))\n", "i", "a[bc]"},
		{"bracket expression first, then `*`", "print $(( m[(I)[ab]*] ))\n", "I", "[ab]*"},
		{"a `.` after the bracket", "print $(( m[(i)a[bc].] ))\n", "i", "a[bc]."},
		{"a POSIX class", "print $(( m[(i)[[:digit:]]*] ))\n", "i", "[[:digit:]]*"},
		{"a space inside the bracket expression", "print $(( m[(i)a[b c]] ))\n", "i", "a[b c]"},
		{"an assignment's arithmetic", "x=$(( m[(i)a[bc]] + 1 ))\n", "i", "a[bc]"},
		{"a flag with an argument", "print $(( m[(n:2:i)a[bc]] ))\n", "n:2:i", "a[bc]"},
		{"arithmetic for header", "for (( i = m[(i)x[0-9]]; i < 3; i++ )) :\n", "i", "x[0-9]"},
		{"inside double quotes", "print \"$(( m[(i)a[bc]] ))\"\n", "i", "a[bc]"},
		{"a range endpoint", "print $(( m[1,(i)a[bc]##] ))\n", "i", "a[bc]##"},
		{"balanced parentheses inside the bracket expression", "print $(( m[(i)a[b()c]] ))\n", "i", "a[b()c]"},
		{"a group before the bracket expression", "print $(( m[(i)(a|b)[bc]] ))\n", "i", "(a|b)[bc]"},
		// The same cut outside arithmetic, reported with a text the
		// error-gated arms above do not name: a length prefix gives
		// "cannot combine multiple parameter expansion operators", a
		// following `:` gives "`:` must be followed by an expression".
		{"length prefix", "print ${#m[(i)a[bc]]}\n", "i", "a[bc]"},
		{"a `:` after the bracket", "print ${m[(i)a[bc]:]}\n", "i", "a[bc]:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "arith.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			flagged := soleFlagsArithm(t, file)
			if flagged.Flags == nil || flagged.Flags.Value != test.flags {
				t.Errorf("Flags = %v, want %q", flagged.Flags, test.flags)
			}
			start := int(flagged.X.Pos().Offset())
			if got := wordSource(test.src, flagged.X); got != test.pattern {
				t.Errorf("pattern = %q, want %q", got, test.pattern)
			}
			if got := wordLiteral(t, flagged.X.(*syntax.Word)); got != test.pattern {
				t.Errorf("pattern literal = %q, want %q (mask not restored)", got, test.pattern)
			}
			if got := test.src[start-1]; got != ')' {
				t.Errorf("byte before pattern = %q, want the flag group's `)`", got)
			}
		})
	}
}

// The arithmetic around the repaired subscript must keep its own operators:
// the mask touches only the pattern's brackets, so `+`, `*`, `?:` and a radix
// constant `2#11` read as they would next to any other operand. `#` is a real
// arithmetic byte (`$(( 2#101 ))` is 5), which is why the repair is keyed on a
// flagged pattern's extent and never on the byte the parser reported.
func TestFlagPatternInArithmeticKeepsSurroundingOperators(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"addition", "print $(( m[(i)a[bc]] + 1 ))\n", "(m[a[bc]] + 1)"},
		{"multiplication of a `##` pattern", "print $(( m[(i)[abc]##] * 2 ))\n", "(m[[abc]##] * 2)"},
		{"radix constant beside the pattern", "print $(( m[(i)a[bc]##] + 2#11 ))\n", "(m[a[bc]##] + 2#11)"},
		{"two cut patterns", "print $(( m[(i)a[bc]] + m[(i)b[xy]] ))\n", "(m[a[bc]] + m[b[xy]])"},
		{"ternary", "print $(( m[(i)a[bc]] ? 1 : 2 ))\n", "(m[a[bc]] ? (1 : 2))"},
		{"comparison in an arithmetic command", "(( m[(i)a[^:]##] == 2 ))\n", "(m[a[^:]##] == 2)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "arith.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var expr syntax.ArithmExpr
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if expr != nil {
					return false
				}
				switch n := node.(type) {
				case *syntax.ArithmExp:
					expr = n.X
				case *syntax.ArithmCmd:
					expr = n.X
				}
				return expr == nil
			})
			if expr == nil {
				t.Fatal("no arithmetic expression in the tree")
			}
			if got := renderArithm(t, test.src, expr); got != test.want {
				t.Errorf("arithmetic = %s, want %s", got, test.want)
			}
		})
	}
}

// renderArithm prints an arithmetic tree with explicit grouping, so a test can
// assert where each operand binds. A subscripted name renders as
// `name[pattern]`, with the pattern taken from the original source.
func renderArithm(t *testing.T, src string, expr syntax.ArithmExpr) string {
	t.Helper()
	switch x := expr.(type) {
	case *syntax.BinaryArithm:
		return "(" + renderArithm(t, src, x.X) + " " + x.Op.String() + " " + renderArithm(t, src, x.Y) + ")"
	case *syntax.Word:
		if len(x.Parts) == 1 {
			if exp, ok := x.Parts[0].(*syntax.ParamExp); ok && exp.Param != nil && exp.Index != nil {
				if flagged, ok := exp.Index.(*syntax.FlagsArithm); ok {
					return exp.Param.Value + "[" + wordSource(src, flagged.X) + "]"
				}
			}
		}
		return wordSource(src, x)
	default:
		t.Fatalf("unexpected arithmetic node %T", expr)
		return ""
	}
}

// When the file still fails after the repair, the error must describe the real
// defect: the one past the pattern, never the premature `]` the parser first
// reported, and never a byte the mask wrote. Both rows pass `zsh -f -n`, which
// does not evaluate arithmetic, and fail when run (the native error is noted
// per row), so they live in testdata/invalid-368-*.txt where the repository's
// `zsh -n` gate never sees them.
func TestFlagPatternInArithmeticReportsTheRealError(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		col     uint
	}{
		// `(( m[(i)a[bc]] x ))`: "bad math expression: operator expected".
		{"testdata/invalid-368-operand-after-pattern.txt", "not a valid arithmetic operator: `x`", 16},
		// `print $(( m[(i)a[bc]] + ))`: "operand expected at end of string".
		{"testdata/invalid-368-missing-operand.txt", "`+` must be followed by an expression", 23},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-368.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v (%T), want a syntax.ParseError", err, err)
			}
			if parseErr.Text != test.text {
				t.Errorf("error text = %q, want %q", parseErr.Text, test.text)
			}
			if parseErr.Pos.Line() != 1 || parseErr.Pos.Col() != test.col {
				t.Errorf("error at %d:%d, want 1:%d", parseErr.Pos.Line(), parseErr.Pos.Col(), test.col)
			}
		})
	}
}

// flagPatternCutBeforeError is the positional gate. It must find a flagged
// pattern the parser cut before the error, and nothing else: no flagged
// opener, a pattern with no bracket expression, a bracket expression that
// closes after the error, and an opener on an earlier line all refuse. The
// helper is tested at its own layer because an over-wide answer here is
// usually caught downstream by holdsFlagPattern, so an end-to-end row would
// pass with the guard deleted.
func TestFlagPatternCutBeforeError(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		seed      int // the offset the parser error points at
		wantOK    bool
		wantStart int
	}{
		// Offsets are measured from the base parser's own errors: the
		// pattern starts at 15, and the cut rows report 20 (the
		// subscript's real `]`, read as an operator) and 23 (the `)`).
		{"cut pattern before the error", "print $(( m[(i)a[bc]] ))", 20, true, 15},
		{"cut pattern, error further on", "print $(( m[(i)a[bc].] ))", 23, true, 15},
		{"no flagged opener", "print $(( m[a[bc]] ))", 17, false, 0},
		{"radix constant, no subscript", "print $(( 2#101 ## ))", 16, false, 0},
		{"flagged pattern without a bracket expression", "print $(( m[(i)abc] x ))", 20, false, 0},
		// The bracket expression's `]` is at 20, after the error at 18.
		{"bracket expression closes after the error", "print $(( m[(i)a[b+c]] ))", 18, false, 0},
		{"opener on an earlier line", "print ${m[(i)a[bc]]}\n(( x y ))", 26, false, 0},
		// A later, whole pattern between the cut and the error does not
		// hide the cut one: the nearest opener with a masked `]` before
		// the error is taken.
		{"nearest cut opener past a whole pattern", "print $(( m[(i)a[bc]] + m[(i)b] x ))", 32, true, 15},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, ok := flagPatternCutBeforeError([]byte(test.src), test.seed)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if ok && start != test.wantStart {
				t.Errorf("start = %d, want %d", start, test.wantStart)
			}
		})
	}
}

// A pattern the repair declines keeps the verdict it has on main. Native Zsh
// counts parentheses to find where `$(( ))` ends, and a quote or a backslash
// changes how it reads the subscript, so each row below is either a native
// parse error, a runtime error of the subscript itself, or valid Zsh whose
// reading the repair cannot decide. Each runs natively as noted.
func TestFlagPatternInArithmeticKeepsUndecidableVerdicts(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		// `zsh -f -n`: parse error near `c]]'.
		{"unopened `)` inside the pattern", "print $(( m[(i)a[b)c]] ))\n"},
		// `zsh -f -n`: parse error; the `(` swallows the closing `))`.
		{"unclosed `(` inside the pattern", "print $(( m[(i)a[b(c]] ))\n"},
		// `zsh -f -n`: parse error near `('; a later `(` must not rebalance.
		{"`)` before `(`", "print $(( m[(i)a[b)(c]] ))\n"},
		// Passes `-n`; run, it is a command substitution and reports
		// `no matches found: m[(i)a]b[x]]`.
		{"escaped `]`", "print $(( m[(i)a\\]b[x]] ))\n"},
		{"escaped `]` inside the bracket expression", "print $(( m[(i)a[b\\]c]] ))\n"},
		{"escaped `[` inside the bracket expression", "print $(( m[(i)a[b\\[c]] ))\n"},
		// A quoted `)` is not counted natively, so a byte count would
		// call this balanced; Zsh reads the quotes, and `-n` passes. It is
		// valid, and stays a known gap rather than a guess.
		{"quoted `)`", "print $(( m[(i)a[b\")\"c]] ))\n"},
		{"quoted bytes", "print $(( m[(i)a[b\"x\"]] ))\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.src), "undecidable.zsh"); err == nil {
				t.Errorf("Parse(%q) unexpectedly succeeded", test.src)
			}
		})
	}
}

// positionalPatternDecidable is tested at its own layer too: several of the
// rows above are also rejected by the chain for other reasons, so an
// end-to-end row alone would not show which check refused it.
func TestPositionalPatternDecidable(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"a[bc]", true},
		{"a[^:]##", true},
		{"a[b()c]", true},
		{"(a|b)[bc]", true},
		{"a[bc](x|y)", true},
		{"a[b)c]", false},
		{"a[b(c]", false},
		{"a[b)(c]", false},
		{"a\\]b[x]", false},
		{"a[\\(]", false},
		{"a[b\"x\"]", false},
		{"a[b'x']", false},
	}
	for _, test := range tests {
		if got := positionalPatternDecidable([]byte(test.pattern)); got != test.want {
			t.Errorf("positionalPatternDecidable(%q) = %v, want %v", test.pattern, got, test.want)
		}
	}
}

// The arithmetic repair is an adapter in the chain, so the chain's composition
// tests cover it with the other adapters' features; this pins it against every
// adapter snippet in both orders, as TestFlagPatternCutComposesWithEveryAdapter
// does for the silent cut.
func TestFlagPatternInArithmeticComposesWithEveryAdapter(t *testing.T) {
	const cut = "print $(( m[(r)a[^:]##] ))"
	for name, snippet := range adapterSnippets {
		t.Run(name, func(t *testing.T) {
			for _, order := range []struct {
				label string
				src   string
			}{
				{"cut first", "#!/usr/bin/env zsh\n" + cut + "\n" + snippet.source + "\n"},
				{"cut second", "#!/usr/bin/env zsh\n" + snippet.source + "\n" + cut + "\n"},
			} {
				file, err := Parse(strings.NewReader(order.src), "compose.zsh")
				if err != nil {
					t.Fatalf("%s: composition must parse:\n%s\nerror: %v", order.label, order.src, err)
				}
				var repaired bool
				syntax.Walk(file.AST(), func(node syntax.Node) bool {
					if flagged, ok := node.(*syntax.FlagsArithm); ok {
						if wordSource(order.src, flagged.X) == "a[^:]##" {
							repaired = true
						}
					}
					return true
				})
				if !repaired {
					t.Errorf("%s: pattern not repaired in:\n%s", order.label, order.src)
				}
			}
		})
	}
}
