package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// zshexpn, Parameter Expansion: the name in `${name}` may be a nested
// command substitution or arithmetic expansion. Native Zsh reads `$((` as
// arithmetic exactly when the first unbalanced `)` is followed by another
// `)` (lex.c, cmd_or_math); mvdan/sh reads every nested `$((` as a command
// substitution. Each row must come back with an arithmetic expansion as the
// nested parameter, over exactly its source bytes, holding no command.
func TestNestedArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// arith is the source text of the nested arithmetic expansion.
		arith string
	}{
		// The #361 rows, which the command reading rejects.
		{"subscript under flags", "print \"${(l:5:)$(( a[1] ))}\"\n", "$(( a[1] ))"},
		{"parameter index", "print \"${(l:5:)$(( a[$e] ))}\"\n", "$(( a[$e] ))"},
		{"the zi autoload shape", "local time=\"${(l:5:: :)$(( ZI[$entry] * 1000 ))%%[,.]*} ms\"\n", "$(( ZI[$entry] * 1000 ))"},
		{"unquoted", "print ${(l:5:)$(( a[1] ))}\n", "$(( a[1] ))"},
		{"no flags", "print ${$(( a[1] ))}\n", "$(( a[1] ))"},
		{"post-increment", "print ${$(( a[1]++ ))}\n", "$(( a[1]++ ))"},
		{"length prefix", "print \"${#$(( a[1] ))}\"\n", "$(( a[1] ))"},
		{"split prefix", "print ${=$(( a[1] ))}\n", "$(( a[1] ))"},
		{"quoted nested parameter", "print ${(l:4:)\"$(( a[1] ))\"}\n", "$(( a[1] ))"},
		{"inside another nested expansion", "print ${${(l:3:)$(( a[1] ))}}\n", "$(( a[1] ))"},
		{"no blanks", "print ${$((a[1]))}\n", "$((a[1]))"},
		{"parenthesized operand", "print ${$(( (1) + a[1] ))}\n", "$(( (1) + a[1] ))"},
		{"expansion operand", "print ${$(( ${a[1]} + a[2] ))}\n", "$(( ${a[1]} + a[2] ))"},
		// Rows the command reading accepts, silently, as the wrong tree.
		{"comparison", "print ${$(( x > 3 ))}\n", "$(( x > 3 ))"},
		{"bitwise and", "print ${$(( x & 1 ))}\n", "$(( x & 1 ))"},
		{"shift", "print ${$(( x << 2 ))}\n", "$(( x << 2 ))"},
		{"plain operand", "print ${$(( x ))}\n", "$(( x ))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			arith := soleNestedArithmetic(t, file.AST())
			start := strings.Index(tc.src, tc.arith)
			if got := int(arith.Left.Offset()); got != start {
				t.Errorf("arithmetic starts at %d, want %d", got, start)
			}
			if got := tc.src[arith.Pos().Offset():arith.End().Offset()]; got != tc.arith {
				t.Errorf("arithmetic spans %q, want %q", got, tc.arith)
			}
			syntax.Walk(arith, func(node syntax.Node) bool {
				switch node.(type) {
				case *syntax.Stmt, *syntax.CallExpr, *syntax.Redirect, *syntax.Subshell:
					t.Errorf("arithmetic holds a command node %T at %v", node, node.Pos())
				}
				return true
			})
		})
	}
}

// The subscript must come back as the arithmetic operand native Zsh reads,
// a subscripted parameter, at its original offsets.
func TestNestedArithmeticKeepsSubscript(t *testing.T) {
	src := "print \"${(l:5:: :)$(( ZI[$entry] * 1000 ))}\"\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	arith := soleNestedArithmetic(t, file.AST())
	product, ok := arith.X.(*syntax.BinaryArithm)
	if !ok || product.Op != syntax.Mul {
		t.Fatalf("arithmetic is %T, want a product", arith.X)
	}
	word, ok := product.X.(*syntax.Word)
	if !ok || len(word.Parts) != 1 {
		t.Fatalf("left operand is %T, want a one-part word", product.X)
	}
	param, ok := word.Parts[0].(*syntax.ParamExp)
	if !ok || param.Param == nil || param.Param.Value != "ZI" || param.Index == nil {
		t.Fatalf("left operand is %#v, want the subscripted parameter ZI", word.Parts[0])
	}
	if got, want := int(param.Param.Pos().Offset()), strings.Index(src, "ZI["); got != want {
		t.Errorf("ZI at offset %d, want %d", got, want)
	}
	if got, want := src[param.Index.Pos().Offset():param.Index.End().Offset()], "$entry"; got != want {
		t.Errorf("index spans %q, want %q", got, want)
	}
}

// Every nested site in a file is resolved, including one inside another.
// Each site is reparsed exactly once: the resolver does not walk into a
// substitution it has claimed, because the reparse of the enclosing span
// resolves the inner sites itself. Walking into it gives the same tree at a
// cost doubling per nesting level (measured: 18 levels took 5.4 s instead
// of 5 ms), so only the count can tell the two apart.
func TestNestedArithmeticEverySite(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"two on one line", "print ${(l:3:)$(( a[1] ))} ${(r:3:)$(( a[2] ))}\n", 2},
		{"one per statement", "print ${$(( a[1] ))}\nprint ${$(( x > 1 ))}\n", 2},
		{"one inside another", "print ${$(( ${$(( a[2] ))} + a[3] ))}\n", 2},
		{"three deep", "print ${$(( ${$(( ${$(( a[1] ))} + 1 ))} + 1 ))}\n", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := arithmeticSpanReparses.Load()
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			if got := int(arithmeticSpanReparses.Load() - before); got != tc.want {
				t.Errorf("span reparses = %d, want one per site (%d)", got, tc.want)
			}
			got := 0
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				switch node := node.(type) {
				case *syntax.ArithmExp:
					got++
				case *syntax.CmdSubst:
					t.Errorf("command substitution left at %v", node.Pos())
				}
				return true
			})
			if got != tc.want {
				t.Errorf("arithmetic expansions = %d, want %d", got, tc.want)
			}
		})
	}
}

// A `$((` whose first unbalanced `)` is not followed by another is a
// command substitution starting with a subshell, in Zsh as in the parser.
func TestNestedArithmeticLeavesCommandSubstitutions(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"subshell then blank", "print ${$((echo a) )}\n"},
		{"two subshells", "print ${$((echo a); (echo b))}\n"},
		{"ordinary substitution", "print ${$(echo a)}\n"},
		{"flagged substitution", "print \"${(l:5:)$(echo a)}\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			subs, ariths := 0, 0
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				switch node.(type) {
				case *syntax.CmdSubst:
					subs++
				case *syntax.ArithmExp:
					ariths++
				}
				return true
			})
			if subs != 1 || ariths != 0 {
				t.Errorf("command substitutions = %d, arithmetic = %d; want 1 and 0", subs, ariths)
			}
		})
	}
}

// An arithmetic body Zsh refuses at runtime (`bad math expression`) keeps
// an error, now the one the same body gets as a top-level `$(( ))`, at its
// original position. The command reading used to accept these.
func TestNestedArithmeticRejectsBadExpressions(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"dangling operator", "print ${$((1 +))}\n", "t.zsh:1:14: `+` must be followed by an expression"},
		{"command text", "print ${$((cd /; ls))}\n", "t.zsh:1:15: `/` must be followed by an expression"},
		{"empty", "print ${$(())}\n", "t.zsh:1:9: `$((` must be followed by an expression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err == nil {
				t.Fatalf("bad arithmetic accepted: %q", tc.src)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

// Outside a nested parameter, `a[1]` at the start of a command is the
// parser's assignment target and keeps its own error.
func TestNestedArithmeticLeavesAssignmentTargets(t *testing.T) {
	for _, src := range []string{"a[1]\n", "print $( a[1] )\n", "print ${$( a[1] )}\n"} {
		_, base := parseTree([]byte(src), "t.zsh")
		_, err := Parse(strings.NewReader(src), "t.zsh")
		if base == nil || err == nil || err.Error() != base.Error() {
			t.Errorf("%q: error = %v, want the parser's %v", src, err, base)
		}
	}
}

// The adapter's masked retry spans lines, so a site across lines must leave
// every later position where it was.
func TestNestedArithmeticAcrossLines(t *testing.T) {
	src := "print ${$((\n  a[1] + 2\n))}\nprint after # c\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}
	arith := soleNestedArithmetic(t, file.AST())
	if got := arith.End().String(); got != "3:3" {
		t.Errorf("arithmetic ends at %s, want 3:3", got)
	}
	stmts := file.AST().Stmts
	if len(stmts) != 2 || stmts[1].Pos().String() != "4:1" {
		t.Fatalf("second statement not at 4:1: %d statements", len(stmts))
	}
	var comments []string
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if comment, ok := node.(*syntax.Comment); ok {
			comments = append(comments, comment.Pos().String())
		}
		return true
	})
	if len(comments) != 1 || comments[0] != "4:13" {
		t.Errorf("comments at %v, want [4:13]", comments)
	}
}

func TestNestedArithmeticEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int // offset past the span, or -1 when not arithmetic
	}{
		{"plain", "$(( 1 ))", 8},
		{"no blanks", "$((1))", 6},
		{"inner group", "$(( (1) + 2 ))", 14},
		{"nested groups", "$(( ((1)) ))", 12},
		{"inner expansion with braces", "$(( ${a[1]} ))", 14},
		{"escaped paren", "$(( 1 \\) ))", 11},
		{"subshell then blank", "$((echo a) )", -1},
		{"two subshells", "$((a); (b))", -1},
		{"unterminated", "$(( 1 ", -1},
		{"quote", "$(( \"1\" ))", -1},
		{"backquote", "$(( `x` ))", -1},
		{"unterminated inner expansion", "$(( ${a ))", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end, ok := nestedArithmeticEnd([]byte(tc.src), 0)
			if !ok {
				end = -1
			}
			if end != tc.want {
				t.Errorf("nestedArithmeticEnd(%q) = %d, want %d", tc.src, end, tc.want)
			}
		})
	}
}

func TestIsNestedParameterStart(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"${$((", true},
		{"${(l:5:)$((", true},
		{"${(l:5:: :)$((", true},
		{"${#$((", true},
		{"${=$((", true},
		{"${~^$((", true},
		{"${\"$((", true},
		{"${(l:4:)\"$((", true},
		{"${+$((", false},
		{"${#+$((", false},
		{"$((", false},
		{"x $((", false},
		{"{$((", false},
		{"${a$((", false},
		{"$x)$((", false},
		{"${(l\n)$((", false},
	} {
		dollar := strings.LastIndex(tc.src, "$((")
		if got := isNestedParameterStart([]byte(tc.src), dollar); got != tc.want {
			t.Errorf("isNestedParameterStart(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// The adapter gates on the error position, so it must not claim an error
// outside a nested arithmetic site.
func TestNestedArithmeticSiteAround(t *testing.T) {
	src := []byte("print ${(l:5:)$(( a[1] ))} $(( b[1] )) ${$(echo (c))}\n")
	inside := strings.Index(string(src), "a[1]")
	if start, end, ok := nestedArithmeticSiteAround(src, inside); !ok || string(src[start:end]) != "$(( a[1] ))" {
		t.Errorf("site around %d = %q, %v; want the nested span", inside, src[start:end], ok)
	}
	for _, offset := range []int{
		strings.Index(string(src), "b[1]"),     // top-level arithmetic
		strings.Index(string(src), "(c)"),      // ordinary substitution
		strings.Index(string(src), "print"),    // outside everything
		strings.Index(string(src), "$(( a[1]"), // the span's own `$`
	} {
		if start, end, ok := nestedArithmeticSiteAround(src, offset); ok {
			t.Errorf("offset %d claimed site %q", offset, src[start:end])
		}
	}
}

// The adapter must verify its retry and the resolver its reparse; both are
// defensive against shapes the scanners never hand them today, so they are
// tested directly.
func TestNestedArithmeticVerification(t *testing.T) {
	tree, err := parseTree([]byte("print ${$(:   )} ${x}\n"), "t.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if !holdsNestedCommandSubstitution(tree, 8, 15) {
		t.Error("the masked site was not found")
	}
	if holdsNestedCommandSubstitution(tree, 8, 14) || holdsNestedCommandSubstitution(tree, 17, 21) {
		t.Error("a span that is not a nested command substitution was accepted")
	}

	// A retry that parses without the placeholder at the site is not
	// trusted: the adapter returns the original error.
	src := []byte("print ${$(( a[1] ))}\n")
	_, firstErr := parseTree(src, "t.zsh")
	elsewhere := func([]byte, string) (*syntax.File, error) {
		return parseTree([]byte("print unrelated\n"), "t.zsh")
	}
	if _, err := parseNestedArithmeticWithParser(src, "t.zsh", firstErr, elsewhere); err != firstErr {
		t.Errorf("unverified retry returned %v, want the original error", err)
	}

	// A span whose reparse is one arithmetic expansion, but not over
	// exactly the span's bytes, is not spliced in.
	src = []byte("print  $(( 1 )) x\n")
	if _, err := parseArithmeticSpan(src, "t.zsh", 6, 15); err == nil {
		t.Error("an arithmetic expansion over different bytes was accepted")
	}
	if _, err := parseArithmeticSpan(src, "t.zsh", 7, 15); err != nil {
		t.Errorf("the exact span was rejected: %v", err)
	}

	src = []byte("print $(echo a) x\n")
	if _, err := parseArithmeticSpan(src, "t.zsh", 6, 15); err == nil {
		t.Error("a command substitution was accepted as an arithmetic span")
	} else if got, want := err.Error(), "t.zsh:1:7: `$((` nested in a parameter expansion must be one arithmetic expansion"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// Where the command reading takes a `#` as a comment, the two readings end
// at different bytes. The tree the base parser built is left as it was
// rather than replaced by a node over a different extent.
func TestNestedArithmeticKeepsDisagreeingExtent(t *testing.T) {
	src := "print ${$((x #))%%\n))}\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if _, ok := node.(*syntax.ArithmExp); ok {
			t.Errorf("arithmetic expansion spliced in at %v", node.Pos())
		}
		return true
	})
}

// `+` is not a nested-parameter prefix: native Zsh rejects `${+$(...)}`
// ("bad substitution"). The command reading already rejects this row, only
// because `a[1]` is an assignment target there, and the adapter must not
// rescue it. Other `+` forms (`${+$(echo 1)}`, `${++$(( x ))}`) are accepted
// by the base parser itself; that is a separate false accept (#363), not
// this adapter's.
func TestNestedArithmeticRejectsPlusPrefix(t *testing.T) {
	src := "print ${+$(( a[1] ))}\n"
	if _, err := Parse(strings.NewReader(src), "t.zsh"); err == nil {
		t.Errorf("invalid Zsh accepted: %q", src)
	}
}

func soleNestedArithmetic(t *testing.T, tree *syntax.File) *syntax.ArithmExp {
	t.Helper()
	var found []*syntax.ArithmExp
	syntax.Walk(tree, func(node syntax.Node) bool {
		exp, ok := node.(*syntax.ParamExp)
		if !ok {
			return true
		}
		nested := exp.NestedParam
		if quoted, ok := nested.(*syntax.DblQuoted); ok && len(quoted.Parts) == 1 {
			nested = quoted.Parts[0]
		}
		if arith, ok := nested.(*syntax.ArithmExp); ok {
			found = append(found, arith)
		}
		return true
	})
	if len(found) != 1 {
		t.Fatalf("nested arithmetic expansions = %d, want 1", len(found))
	}
	return found[0]
}
