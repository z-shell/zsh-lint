package syntax_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax/typedjson"
)

// Issue #693. Zsh's par_cond_2 (Src/parse.c) lexes the right operand of a
// binary condition with parentheses as part of the word, so a glob group in
// the right operand of a numeric or file comparison nests bare parentheses,
// as in a `-NAME` condition operand (#484). Native verdicts use
// newline-terminated files and zsh -f -n with Zsh 5.9.2.
// https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html
func TestZshCondOperandGroup(t *testing.T) {
	for _, src := range []string{
		"[[ 5 -eq ((o+4)) ]]",
		"[[ 5 -eq (( o + 4 )) ]]",
		"[[ 5 -eq (((o))) ]]",
		"[[ 5 -eq ((o+4))x ]]",
		"[[ 5 -eq x((o)) ]]",
		"[[ 5 -eq x((o))y ]]",
		"[[ 5 -eq ((1|4)) ]]",
		"[[ 5 -eq ((o)+(4)) ]]",
		"[[ 5 -eq ((a b)) ]]",
		"[[ 5 -eq ((x)(y)) ]]",
		"[[ 5 -eq ((x))((y)) ]]",
		"[[ 5 -eq ((a)|(b)) ]]",
		"[[ 5 -eq ((<1-2>)) ]]",
		"[[ 5 -eq ((<(x))) ]]",
		"[[ 5 -eq (($x)) ]]",
		"[[ 5 -eq ((${x})) ]]",
		"[[ 5 -eq (($(echo 1))) ]]",
		"[[ 5 -eq ((${x:-(})) ]]",
		"[[ 5 -eq ((\\))) ]]",
		"[[ 5 -eq ((')')) ]]",
		"[[ 5 -eq ((\")\")) ]]",
		"[[ 5 -eq ((#q)) ]]",
		"[[ 5 -ne ((o+4)) ]]",
		"[[ 5 -lt ((o+4)) ]]",
		"[[ 5 -gt (( o + 4 )) ]]",
		"[[ 5 -le ((o+4)) ]]",
		"[[ 5 -ge ((o+4)) ]]",
		"[[ x -nt ((a)) ]]",
		"[[ x -ot ((a)) ]]",
		"[[ x -ef ((a)) ]]",
		"[[ 5 -eq ((o+4)) && 1 -eq 1 ]]",
		"[[ 1 -eq 1 || 5 -eq ((o+4)) ]]",
		"[[ ! 5 -eq ((o+4)) ]]",
		"[[ ( 5 -eq ((o+4)) ) ]]",
		"[[ 5 -eq ((o+\n4)) ]]",
		// Forms that parsed before keep parsing.
		"[[ 5 -eq (o+4) ]]",
		"[[ 5 -eq ( o + 4 ) ]]",
		"[[ 5 -eq (1|4) ]]",
		"[[ 5 -eq (x)(y) ]]",
		"[[ 5 -eq $((o+4)) ]]",
		"[[ 5 -eq (<(x)) ]]",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var printed bytes.Buffer
			if err := syntax.NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			again, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(&printed, "")
			if err != nil {
				t.Fatalf("printed source %q does not parse: %v", printed.String(), err)
			}
			if diff := cmp.Diff(f, again, cmpopts.IgnoreTypes(syntax.Pos{})); diff != "" {
				t.Fatalf("printing changed the tree (-want +got):\n%s", diff)
			}
			var encoded bytes.Buffer
			if err := typedjson.Encode(&encoded, f); err != nil {
				t.Fatal(err)
			}
			decoded, err := typedjson.Decode(&encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f, decoded) {
				t.Fatal("typedjson changed the tree")
			}
		})
	}
}

// The right operand stays one word holding the whole group, with its
// source position.
func TestZshCondOperandGroupTree(t *testing.T) {
	for _, tc := range []struct {
		src, op, operand string
		col              uint
	}{
		{"[[ 5 -eq ((o+4)) ]]", "-eq", "((o+4))", 10},
		{"[[ 5 -gt (( a + 2 )) ]]", "-gt", "(( a + 2 ))", 10},
		{"[[ 5 -eq x((o))y ]]", "-eq", "x((o))y", 10},
		{"[[ x -nt ((a)) ]]", "-nt", "((a))", 10},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			test, ok := f.Stmts[0].Cmd.(*syntax.TestClause)
			if !ok {
				t.Fatalf("command is %T, want *syntax.TestClause", f.Stmts[0].Cmd)
			}
			bin, ok := test.X.(*syntax.BinaryTest)
			if !ok {
				t.Fatalf("condition is %T, want *syntax.BinaryTest", test.X)
			}
			if got := bin.Op.String(); got != tc.op {
				t.Fatalf("operator = %s, want %s", got, tc.op)
			}
			y, ok := bin.Y.(*syntax.Word)
			if !ok {
				t.Fatalf("right operand is %T, want *syntax.Word", bin.Y)
			}
			if got := y.Lit(); got != tc.operand {
				t.Fatalf("right operand = %q, want %q", got, tc.operand)
			}
			if got := y.Pos().Col(); got != tc.col {
				t.Fatalf("right operand column = %d, want %d", got, tc.col)
			}
			if got := y.End().Col(); got != tc.col+uint(len(tc.operand)) {
				t.Fatalf("right operand end column = %d, want %d", got, tc.col+uint(len(tc.operand)))
			}
		})
	}
}

// A `&&` ends the word inside an open group and is then the condition's own
// operator, as in Zsh's lexer: the operand keeps the group's `(` and every
// blank before the `&&`. With `zsh -f`, `[[ '(a ' -ef (a && 1 == 1 ]]` is true
// for an existing file named `(a `. Zsh -f -n accepts these; -eq reports a bad
// math expression only when it runs. The printer adds a blank before `&&`, so
// these rows do not round-trip; the `==` operand `[[ a == (b && c ]]` printed
// the same way before #693.
func TestZshCondOperandGroupEndsAtAndAnd(t *testing.T) {
	for _, tc := range []struct{ src, operand string }{
		{"[[ 5 -eq ((o+4) && 1 -eq 1 ]]", "((o+4) "},
		{"[[ 5 -eq (o && x ]]", "(o "},
		{"[[ 5 -eq (o  && x ]]", "(o  "},
		{"[[ x -nt (a && y ]]", "(a "},
		{"[[ x -nt (a&& y ]]", "(a"},
		{"[[ a -foo (b && c ]]", "(b "},
		{"[[ a -foo ((b) && c ]]", "((b) "},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			and, ok := f.Stmts[0].Cmd.(*syntax.TestClause).X.(*syntax.BinaryTest)
			if !ok || and.Op != syntax.AndTest {
				t.Fatalf("condition is %#v, want an && test", f.Stmts[0].Cmd.(*syntax.TestClause).X)
			}
			var y *syntax.Word
			switch x := and.X.(type) {
			case *syntax.BinaryTest:
				y, _ = x.Y.(*syntax.Word)
			case *syntax.ModuleTest:
				y = x.Args[1]
			}
			if y == nil || y.Lit() != tc.operand {
				t.Fatalf("left condition is %#v, want right operand %q", and.X, tc.operand)
			}
		})
	}
}

// Sources native Zsh rejects stay rejected, including the unbalanced groups
// that parsed before #693.
func TestZshCondOperandGroupReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ 5 -eq ((o+4) ]]", "1:10: reached EOF without matching `(` with `)`"},
		{"[[ 5 -eq (( o + 4 ) ]]", "1:10: reached EOF without matching `(` with `)`"},
		{"[[ 5 -eq ((o) ]]", "1:10: reached EOF without matching `(` with `)`"},
		{"[[ x -nt ((a) ]]", "1:10: reached EOF without matching `(` with `)`"},
		{"[[ 5 -eq (o+4)) ]]", "1:1: reached `)` without matching `[[` with `]]`"},
		{"[[ 5 -eq ((o+4)) x ]]", "1:18: not a valid test operator: `x`"},
		{"[[ 5 -eq ((o+4) || 1 -eq 1 ]]", "1:10: reached EOF without matching `(` with `)`"},
		{"[[ 5 -eq (o & x) ]]", "1:13: a condition glob group cannot contain `&`"},
		{"[[ 5 -eq ((1&2)) ]]", "1:13: a condition glob group cannot contain `&`"},
		{"[[ 5 -eq ((1;2)) ]]", "1:13: a condition glob group cannot contain `;`"},
		// `<2` is not a numeric glob, so the `)` after it is reported.
		{"[[ 5 -eq ((1<2)) ]]", "1:15: a condition glob group cannot contain `)`"},
		{"[[ 5 -eq ((1>2)) ]]", "1:13: a condition glob group cannot contain `>`"},
		{"[[ 5 -eq ((x)()) ]]", "1:14: a condition glob group cannot contain `()`"},
		{"[[ ((1)) -eq 1 ]]", "1:10: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ -n ((x)) ]]", "1:7: a condition operand cannot start with `(`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// The change is Zsh-only: Bash keeps upstream's reading of the same source.
func TestZshCondOperandGroupBashUnchanged(t *testing.T) {
	for _, src := range []string{"[[ 5 -eq ((o+4)) ]]", "[[ 5 -eq ((o+4) ]]", "[[ x -nt ((a)) ]]"} {
		_, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src+"\n"), "")
		if err == nil {
			t.Errorf("bash accepted %q", src)
		}
	}
}
