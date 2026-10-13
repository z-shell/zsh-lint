package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// Zsh substitutes expansions before it evaluates arithmetic, so an expansion
// at a blank may supply the operator or operand between two others
// (zsh-lint #672).
// https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
func TestZshDynamicArithmeticOperands(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		items int    // items of the outermost dynamic sequence
		print string // printed form, when it differs from expr
	}{
		{`1 $op 2`, 3, ""},
		{`$#dirstack $linepath`, 2, ""},
		{`$_base $1`, 2, ""},
		{`1 "$op" 2`, 3, ""},
		{`1 ${op} 2 * 3`, 3, ""},
		{`1 + 2 $op 3`, 3, ""},
		{`1 $op $op 2`, 4, ""},
		{`(1) $op (2)`, 3, ""},
		{`1 $op (2)`, 3, ""},
		{`1 $(print +) 2`, 3, ""},
		{"1 `print +` 2", 3, "1 $(print +) 2"},
		{`1 $op $(( 2 ))`, 3, `1 $op $((2))`},
		{`$op $[ 2 ]`, 2, `$op $[2]`},
		{`x = 1 $op 2`, 3, ""},
		{`-$a 2`, 2, ""},
		{`2 $a++`, 2, ""},
		{`$op !x`, 2, `$op ! x`},
		{`$op ~x`, 2, ""},
		{`$op #x`, 2, ""},
		{`$op a$b`, 2, ""},
		{`$op x[1]`, 2, ""},
		{`a[1] $op a[2]`, 3, ""},
		{`1 $op [#16]`, 2, ""},
		{`$op [#16] 2`, 2, ""},
		{`$# $op 2`, 3, ""},
		{`1 ${(l:2::+:)#op}`, 2, ""},
		{`1 ${op:-+} 2`, 3, ""},
		{`1 "$op x"`, 2, ""},
		{"1 $op\n2", 3, "1 $op \\\n2"},
	} {
		for _, form := range []struct{ src, printed string }{
			{"(( %s ))", "((%s))"},
			{"print $(( %s ))", "print $((%s))"},
		} {
			src := strings.Replace(form.src, "%s", tc.expr, 1)
			t.Run(src, func(t *testing.T) {
				f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\nprint after\n"), "")
				if err != nil {
					t.Fatal(err)
				}
				if len(f.Stmts) != 2 {
					t.Fatalf("following statement lost: %d statements", len(f.Stmts))
				}
				var dyn *DynamicArithm
				params := 0
				Walk(f.Stmts[0], func(n Node) bool {
					switch n := n.(type) {
					case *DynamicArithm:
						if dyn == nil {
							dyn = n
						}
					case *ParamExp:
						params++
					}
					return true
				})
				if dyn == nil {
					t.Fatal("no dynamic arithmetic sequence")
				}
				if len(dyn.Items) != tc.items {
					t.Fatalf("got %d items, want %d", len(dyn.Items), tc.items)
				}
				if params == 0 && !strings.Contains(tc.expr, "print +") {
					t.Fatal("expansion lost to walker")
				}
				start := strings.Index(src, tc.expr)
				first, last := dyn.Items[0], dyn.Items[len(dyn.Items)-1]
				if dyn.Pos() != first.Pos() || dyn.End() != last.End() {
					t.Fatal("sequence positions differ from its items")
				}
				if strings.HasPrefix(tc.expr, "x = ") {
					start += len("x = ")
				}
				if strings.HasPrefix(tc.expr, "1 + ") {
					start += len("1 + ")
				}
				if got := int(dyn.Pos().Offset()); got != start {
					t.Fatalf("sequence starts at %d, want %d", got, start)
				}
				var printed bytes.Buffer
				if err := NewPrinter().Print(&printed, f); err != nil {
					t.Fatal(err)
				}
				want := tc.expr
				if tc.print != "" {
					want = tc.print
				}
				want = strings.Replace(form.printed, "%s", want, 1) + "\nprint after\n"
				if printed.String() != want {
					t.Fatalf("printed %q, want %q", printed.String(), want)
				}
				if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed.String()), ""); err != nil {
					t.Fatalf("printed source rejected: %v", err)
				}
			})
		}
	}
}

// Operands joined by a blank with no expansion at that blank stay errors,
// with the message and position upstream gives them.
func TestZshDynamicArithmeticOperandsRejected(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{`(( 1 2 ))`, "1:6: not a valid arithmetic operator: `2`"},
		{`(( a b ))`, "1:6: not a valid arithmetic operator: `b`"},
		{`(( (1) (2) ))`, "1:8: not a valid arithmetic operator: `(`"},
		{`(( a[1] b ))`, "1:9: not a valid arithmetic operator: `b`"},
		{`(( 1 $op 2 3 ))`, "1:12: not a valid arithmetic operator: `3`"},
		{`(( 1 "+" 2 ))`, "1:6: not a valid arithmetic operator: `\"`"},
		{`(( 1 "x$op" 2 ))`, "1:6: not a valid arithmetic operator: `\"`"},
		{`(( 1 "$op x" 2 ))`, "1:14: not a valid arithmetic operator: `2`"},
		{`(( 1 "" 2 ))`, "1:6: not a valid arithmetic operator: `\"`"},
		{`(( 1 [#16] 2 ))`, "1:12: not a valid arithmetic operator: `2`"},
		// A length, a ${+name} test and the numeric special parameters
		// expand to a number, which supplies no operator.
		{`(( 1 ${#op} 2 ))`, "1:6: not a valid arithmetic operator: `${`"},
		{`(( 1 ${+op} 2 ))`, "1:6: not a valid arithmetic operator: `${`"},
		{`(( 1 $# 2 ))`, "1:6: not a valid arithmetic operator: `$`"},
		{`(( 1 ${?} 2 ))`, "1:6: not a valid arithmetic operator: `${`"},
		// Quoted text is part of the arithmetic text: blanks inside it need
		// an expansion beside them too, and single quotes are never valid.
		{`(( 1 $op "2 3" ))`, "1:10: not a valid arithmetic operator: `\"`"},
		{`(( "2 3" $op 1 ))`, "1:10: not a valid arithmetic operator: `$`"},
		{`(( 1 $op a'x' ))`, "1:10: not a valid arithmetic operator: `a`"},
		{`(( $op !"2 3" ))`, "1:8: not a valid arithmetic operator: `!`"},
		{`(( $op "2 3" [#16] ))`, "1:8: not a valid arithmetic operator: `\"`"},
		{`(( 2 "x"++ ))`, "1:6: not a valid arithmetic operator: `\"`"},
		{`(( 1 $(( 2 )) ))`, "1:6: not a valid arithmetic operator: `$((`"},
		{`(( 1 $[ 2 ] ))`, "1:6: not a valid arithmetic operator: `$`"},
		{`(( $op 'x' ))`, "1:8: not a valid arithmetic operator: `'`"},
		{`(( $op $'1' ))`, "1:8: not a valid arithmetic operator: `$'`"},
		{`(( 1 $"$op" 2 ))`, "1:6: not a valid arithmetic operator: `$`"},
		// An error inside an expansion that may be an operand is reported
		// where it stands, as in any other operand.
		{`(( 1 ${op ))`, "1:10: not a valid parameter expansion operator: ` `"},
		{`(( $op 1 ${op ))`, "1:14: not a valid parameter expansion operator: ` `"},
		{`(( $op ))`, ""},
		{`print $(( 1 2 ))`, "1:13: not a valid arithmetic operator: `2`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tc.src), "")
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.err {
				t.Fatalf("got error %q, want %q", got, tc.err)
			}
		})
	}
}

// Other languages keep upstream's grammar.
func TestZshDynamicArithmeticOperandsDialectBoundary(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn} {
		for _, src := range []string{`(( 1 $op 2 ))`, `echo $(( $a $b ))`} {
			if _, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), ""); err == nil {
				t.Fatalf("%s accepted %q", lang, src)
			}
		}
	}
	// A let argument and an associative subscript keep their grammar.
	for _, src := range []string{`let '1 $op 2'`, `print ${a[1 $op 2]}`} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err != nil {
			continue
		}
		Walk(f, func(n Node) bool {
			if _, ok := n.(*DynamicArithm); ok {
				t.Fatalf("%q read a dynamic arithmetic sequence", src)
			}
			return true
		})
	}
}
