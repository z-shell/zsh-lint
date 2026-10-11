package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// Zsh arithmetic output directives and character values, initiative #612.
// https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
func TestZshArithmeticFormatAndCharacter(t *testing.T) {
	for _, expr := range []string{
		`[#16] 255`, `[##16] #key`, `[#16_4] 65536`, `[#_] 123456`,
		`1 + [#16] 2`, `1 [#16] + 2`, `[#16]`, `[#16][##8] 255`, `key [#16] = 32`,
		`#key`, `1 + #key`, `#key[1]`, `#$name`, `#key = 2`, `++#key`, `#key++`,
		`#key [#16]`, `##a [#16] + 1`, `##a[#16] + 1`, `[#16] ##a [#8] + 1`,
		`##a`, `##*`, `##+`, `##^A`, `##\M-\C-x`, `##\n`, `##$MATCH`,
		`##\x41 + 1`, `##\u0041 + 1`, `##\U00000041 + 1`, `##\123 + 1`,
		`##\xAf + 1`, `##\xaf + 1`, `##\xAF + 1`, `##\u03b1 + 1`, `##\U0001F600 + 1`,
		`##\007 + 1`, `##\377 + 1`, `##\x4 + 1`, `##\M-a`, `##\C-a`,
		`key++ [#16]`, `key--[##16]`, `key++ [#16][#8] + 1`,
		`key [#8]++ [##16]`, `#key-- [#16]`, `key[1]++ [#16]`,
	} {
		t.Run(expr, func(t *testing.T) {
			for _, src := range []string{"(( " + expr + " ))\n", "print $(( " + expr + " ))\n"} {
				f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err := NewPrinter().Print(&out, f); err != nil {
					t.Fatal(err)
				}
				if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(out.String()), ""); err != nil {
					t.Fatalf("printed source rejected: %q: %v", out.String(), err)
				}
			}
		})
	}
}

func TestZshArithmeticFormatDialectAndErrors(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn} {
		for _, src := range []string{"(( [##16] 255 ))", "(( key++ [#16] ))"} {
			if _, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), ""); err == nil {
				t.Fatalf("%s accepted Zsh output format: %s", lang, src)
			}
		}
	}
	f, err := NewParser(Variant(LangMirBSDKorn)).Parse(strings.NewReader("(( # key ))"), "")
	if err != nil || !f.Stmts[0].Cmd.(*ArithmCmd).Unsigned {
		t.Fatalf("mksh unsigned grammar changed: %v", err)
	}
	for _, src := range []string{"(( [##16] 255 )", "(( [#16] + ))", "(( ##) ))", "(( ##( ))", "(( ##a = 2 ))", "(( key++ [#16]++ ))", "(( 1++ [#16] ))", "(( key++ [#16 ))"} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err == nil {
			t.Fatalf("malformed arithmetic accepted: %s", src)
		}
	}
}

func TestZshArithmeticFormatPostAndWalk(t *testing.T) {
	src := "(( key [#16] = 32 ))\n"
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	x := f.Stmts[0].Cmd.(*ArithmCmd).X.(*BinaryArithm).X.(*OutputBaseArithm)
	if !x.Post || x.Pos().Offset() != 3 || x.End().Offset() != 12 || x.Format.Value != "#16" {
		t.Fatalf("post-format source positions lost: %+v", x)
	}
	var lits []string
	Walk(x, func(n Node) bool {
		if lit, ok := n.(*Lit); ok {
			lits = append(lits, lit.Value)
		}
		return true
	})
	if strings.Join(lits, "/") != "key/#16" {
		t.Fatalf("walker lost source order: %v", lits)
	}
	var printed bytes.Buffer
	if err := NewPrinter().Print(&printed, f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(printed.String(), "key [#16]") {
		t.Fatalf("printer turned output format into a subscript: %s", &printed)
	}
	f, err = NewParser(Variant(LangZsh)).Parse(strings.NewReader("(( key[#16] ))"), "")
	if err != nil {
		t.Fatal(err)
	}
	index := f.Stmts[0].Cmd.(*ArithmCmd).X.(*Word).Parts[0].(*ParamExp)
	if index.Index == nil || index.Param.Value != "key" {
		t.Fatalf("glued name subscript changed to output format: %+v", index)
	}
}

func TestZshArithmeticFormatPositionsAndSubscripts(t *testing.T) {
	for _, expr := range []string{`[#16] 255`, `[#16]`, `#`, `##a`} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("(( "+expr+" ))"), "")
		if err != nil {
			t.Fatal(err)
		}
		x := f.Stmts[0].Cmd.(*ArithmCmd).X
		if x.Pos().Offset() != 3 || x.End().Offset() != uint(3+len(expr)) {
			t.Fatalf("%s range = %d:%d", expr, x.Pos().Offset(), x.End().Offset())
		}
	}
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("(( #key[#16] ))"), "")
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Cmd.(*ArithmCmd).X.(*CharArithm)
	index := c.Value.Parts[0].(*ParamExp)
	if !index.Short || index.Param.Value != "key" || index.Index == nil {
		t.Fatalf("glued character name subscript changed: %+v", index)
	}
	if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("(( #key [1] ))"), ""); err == nil {
		t.Fatal("spaced bracket accepted as a character-name subscript")
	}
}

func TestZshArithmeticFormatExpansionsRemainUnsupported(t *testing.T) {
	// These native-valid forms need typed expansion children before acceptance.
	for _, src := range []string{"print $(( [#$base] 255 ))", "print $(( [#$(print 16)] 255 ))", "print $(( [#`print 16`] 255 ))"} {
		_, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err == nil || !strings.Contains(err.Error(), "shell expansions in arithmetic output formats") {
			t.Fatalf("output-format expansion hidden as literal text: %v", err)
		}
	}
}

func TestZshArithmeticCharacterKeepsFollowingOperator(t *testing.T) {
	for _, literal := range []string{`a`, `*`, `^A`, `\n`, `\a`, `\M-a`, `\C-a`, `\x4`, `\xAF`, `\xaf`, `\u03b1`, `\U0001F600`, `\0`, `\7`, `\12`, `\007`, `\377`} {
		src := "(( ##" + literal + "-1 ))"
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		sub, ok := f.Stmts[0].Cmd.(*ArithmCmd).X.(*BinaryArithm)
		if !ok || sub.Op != Sub {
			t.Fatalf("character swallowed following subtraction: %s", src)
		}
		char, ok := sub.X.(*CharArithm)
		if !ok || !char.Literal || char.Value.Lit() != literal || char.End().Offset() != uint(5+len(literal)) {
			t.Fatalf("literal character boundary changed: %s: %+v", src, char)
		}
	}
}
