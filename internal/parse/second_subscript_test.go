package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// nodeText returns the source bytes a node spans, so a position assertion
// and a content assertion are one check.
func nodeText(src string, node syntax.Node) string {
	return src[node.Pos().Offset():node.End().Offset()]
}

func TestParseSecondSubscript(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		param      string
		first      string
		subscripts []string
		exp        syntax.ParExpOperator
		length     bool
	}{
		{name: "element of element", src: "print ${a[1][2]}\n", param: "a", first: "1", subscripts: []string{"2"}},
		{name: "range of element", src: "print ${a[b][1,50]}\n", param: "a", first: "b", subscripts: []string{"1,50"}},
		{name: "quoted hyphenated key", src: "print \"${sice[ps-on-unload][1,50]}\"\n", param: "sice", first: "ps-on-unload", subscripts: []string{"1,50"}},
		{name: "computed first index", src: "argv[$argi]=(-n ${argv[$argi][2,-1]})\n", param: "argv", first: "$argi", subscripts: []string{"2,-1"}},
		{name: "operator after", src: "print ${sice[ps-on-unload][51]:+x}\n", param: "sice", first: "ps-on-unload", subscripts: []string{"51"}, exp: syntax.AlternateUnsetOrNull},
		{name: "default after", src: "print ${a[b][1]:-none}\n", param: "a", first: "b", subscripts: []string{"1"}, exp: syntax.DefaultUnsetOrNull},
		{name: "range first", src: "print ${a[1,2][3]}\n", param: "a", first: "1,2", subscripts: []string{"3"}},
		{name: "three subscripts", src: "print ${a[1][2][3]}\n", param: "a", first: "1", subscripts: []string{"2", "3"}},
		{name: "ranges throughout", src: "print ${a[1,2][3,4][5,6]}\n", param: "a", first: "1,2", subscripts: []string{"3,4", "5,6"}},
		{name: "length prefix", src: "print ${#a[b][1]}\n", param: "a", first: "b", subscripts: []string{"1"}, length: true},
		{name: "flagged param", src: "print ${(f)a[b][1]}\n", param: "a", first: "b", subscripts: []string{"1"}},
		{name: "negative index", src: "print ${a[b][-1]}\n", param: "a", first: "b", subscripts: []string{"-1"}},
		{name: "expansion index", src: "print ${a[b][${i}]}\n", param: "a", first: "b", subscripts: []string{"${i}"}},
		{name: "nested subscript", src: "print ${a[b][$line[1]]}\n", param: "a", first: "b", subscripts: []string{"$line[1]"}},
		{name: "flagged second subscript", src: "print ${a[b][(r)x*]}\n", param: "a", first: "b", subscripts: []string{"(r)x*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tt.src), "second-subscript.zsh")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			seconds := file.SecondSubscripts()
			if len(seconds) != 1 {
				t.Fatalf("SecondSubscripts() = %d entries, want 1", len(seconds))
			}
			exp := seconds[0].Expansion
			if exp.Param.Value != tt.param {
				t.Fatalf("Param = %q, want %q", exp.Param.Value, tt.param)
			}
			if got := nodeText(tt.src, exp.Index); got != tt.first {
				t.Fatalf("Index text = %q, want %q", got, tt.first)
			}
			if len(seconds[0].Subscripts) != len(tt.subscripts) {
				t.Fatalf("Subscripts = %d, want %d", len(seconds[0].Subscripts), len(tt.subscripts))
			}
			for i, want := range tt.subscripts {
				if got := nodeText(tt.src, seconds[0].Subscripts[i]); got != want {
					t.Errorf("Subscripts[%d] text = %q, want %q", i, got, want)
				}
			}
			if exp.Exp == nil && tt.exp != 0 || exp.Exp != nil && exp.Exp.Op != tt.exp {
				t.Errorf("Exp = %v, want operator %v", exp.Exp, tt.exp)
			}
			if exp.Length != tt.length {
				t.Errorf("Length = %v, want %v", exp.Length, tt.length)
			}
			if got := tt.src[exp.Pos().Offset():exp.End().Offset()]; !strings.HasPrefix(got, "${") || !strings.HasSuffix(got, "}") {
				t.Errorf("expansion span = %q, want the whole ${...}", got)
			}
		})
	}
}

// The second subscript keeps the typed arithmetic shape the first one has:
// a range is a comma BinaryArithm over its bounds, with the parser's own
// operator position.
func TestSecondSubscriptKeepsArithmeticShape(t *testing.T) {
	src := "print ${argv[$argi][2,-1]}\n"
	file, err := Parse(strings.NewReader(src), "second-subscript.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	seconds := file.SecondSubscripts()
	if len(seconds) != 1 || len(seconds[0].Subscripts) != 1 {
		t.Fatalf("SecondSubscripts() = %+v, want one entry with one subscript", seconds)
	}
	bin, ok := seconds[0].Subscripts[0].(*syntax.BinaryArithm)
	if !ok || bin.Op != syntax.Comma {
		t.Fatalf("subscript = %T %v, want a comma BinaryArithm", seconds[0].Subscripts[0], seconds[0].Subscripts[0])
	}
	if got := nodeText(src, bin.X); got != "2" {
		t.Errorf("X = %q, want 2", got)
	}
	if got := nodeText(src, bin.Y); got != "-1" {
		t.Errorf("Y = %q, want -1", got)
	}
	if bin.OpPos.Col() != 22 || src[bin.OpPos.Offset()] != ',' {
		t.Errorf("OpPos = %v (%q), want column 22 on the comma", bin.OpPos, src[bin.OpPos.Offset()])
	}
	if _, ok := seconds[0].Subscripts[0].(*syntax.BinaryArithm).Y.(*syntax.UnaryArithm); !ok {
		t.Errorf("Y = %T, want a UnaryArithm", bin.Y)
	}
	index, ok := seconds[0].Expansion.Index.(*syntax.Word)
	if !ok || len(index.Parts) != 1 {
		t.Fatalf("Index = %T, want a one-part Word", seconds[0].Expansion.Index)
	}
	if _, ok := index.Parts[0].(*syntax.ParamExp); !ok {
		t.Errorf("Index part = %T, want the $argi expansion", index.Parts[0])
	}
}

// A native range comma is not a subscript boundary: the source bytes at the
// operator decide, so `${a[1,50]}` keeps its index and records nothing.
func TestSecondSubscriptLeavesNativeCommaAlone(t *testing.T) {
	for _, src := range []string{
		"print ${a[1,50]}\n",
		"print ${a[b, 1]}\n",
		"print ${a[b]}\n",
		"a[1,2]=x\n",
	} {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src), "second-subscript.zsh")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if seconds := file.SecondSubscripts(); len(seconds) != 0 {
				t.Fatalf("SecondSubscripts() = %+v, want none", seconds)
			}
		})
	}
}

func TestSecondSubscriptRejectsInvalidSources(t *testing.T) {
	for _, fixture := range []string{
		"testdata/invalid-215-unterminated-second-subscript.txt",
		"testdata/invalid-215-double-close-before-second-subscript.txt",
	} {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-215.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) || parseErr.Pos.Line() != 1 {
				t.Fatalf("Parse() error = %v, want a parse error on line 1", err)
			}
		})
	}
}

// A later, unrelated error keeps its true position after the masked retry.
func TestSecondSubscriptPreservesLaterErrorPosition(t *testing.T) {
	src := "print ${a[b][1,50]}\nif true; then\n"
	_, err := Parse(strings.NewReader(src), "second-subscript.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
	if parseErr.Pos.Line() != 3 && parseErr.Pos.Line() != 2 {
		t.Fatalf("error at %v, want the unterminated if on line 2 or its EOF", parseErr.Pos)
	}
	if parseErr.Text == invalidSecondSubscript {
		t.Fatalf("error = %v, the second subscript was not accepted", err)
	}
}

// The printed tree is the closest typed shape: the first subscript stays in
// the expansion and every later one lives only in the metadata.
func TestSecondSubscriptPrintsFirstSubscriptOnly(t *testing.T) {
	file, err := Parse(strings.NewReader("print ${a[b][1,50]:-x}\n"), "second-subscript.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print: %v", err)
	}
	if got := rendered.String(); !strings.Contains(got, "${a[b]:-x}") {
		t.Fatalf("printed = %q, want ${a[b]:-x}", got)
	}
}
