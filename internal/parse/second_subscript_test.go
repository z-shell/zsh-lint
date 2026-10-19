package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
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
		isSet      bool
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
		{name: "existence prefix", src: "print ${+a[b][1]}\n", param: "a", first: "b", subscripts: []string{"1"}, isSet: true},
		{name: "flagged param", src: "print ${(f)a[b][1]}\n", param: "a", first: "b", subscripts: []string{"1"}},
		{name: "negative index", src: "print ${a[b][-1]}\n", param: "a", first: "b", subscripts: []string{"-1"}},
		{name: "expansion index", src: "print ${a[b][${i}]}\n", param: "a", first: "b", subscripts: []string{"${i}"}},
		{name: "nested subscript", src: "print ${a[b][$line[1]]}\n", param: "a", first: "b", subscripts: []string{"$line[1]"}},
		{name: "flagged second subscript", src: "print ${a[b][(r)x*]}\n", param: "a", first: "b", subscripts: []string{"(r)x*"}},
		{name: "bracket pattern before a later subscript", src: "print ${a[(i)[x]][2]}\n", param: "a", first: "(i)[x]", subscripts: []string{"2"}},
		{name: "flagged subscripts throughout", src: "print ${a[(i)x][(r)[y]][1]}\n", param: "a", first: "(i)x", subscripts: []string{"(r)[y]", "1"}},
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
			if exp.IsSet != tt.isSet {
				t.Errorf("IsSet = %v, want %v", exp.IsSet, tt.isSet)
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

// A second subscript nested inside another expansion's later subscript is
// split as well: the walk collects every expansion before any index is
// re-pointed, so the inner expansion is still reachable.
func TestSecondSubscriptSplitsNestedExpansions(t *testing.T) {
	src := "print ${a[1][${b[2][3]}]}\n"
	file, err := Parse(strings.NewReader(src), "second-subscript.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	seconds := file.SecondSubscripts()
	if len(seconds) != 2 {
		t.Fatalf("SecondSubscripts() = %d entries, want 2", len(seconds))
	}
	want := []struct {
		param, first, second string
	}{
		{"a", "1", "${b[2][3]}"},
		{"b", "2", "3"},
	}
	for i, w := range want {
		got := seconds[i]
		if got.Expansion.Param.Value != w.param {
			t.Errorf("[%d] Param = %q, want %q", i, got.Expansion.Param.Value, w.param)
		}
		if text := nodeText(src, got.Expansion.Index); text != w.first {
			t.Errorf("[%d] Index text = %q, want %q", i, text, w.first)
		}
		if len(got.Subscripts) != 1 {
			t.Fatalf("[%d] Subscripts = %d, want 1", i, len(got.Subscripts))
		}
		if text := nodeText(src, got.Subscripts[0]); text != w.second {
			t.Errorf("[%d] Subscripts[0] text = %q, want %q", i, text, w.second)
		}
	}
	var inner []*syntax.ParamExp
	syntax.Walk(seconds[0].Subscripts[0], func(node syntax.Node) bool {
		if exp, ok := node.(*syntax.ParamExp); ok {
			inner = append(inner, exp)
		}
		return true
	})
	if len(inner) != 1 || inner[0] != seconds[1].Expansion {
		t.Errorf("outer second subscript holds %d expansions, want the inner entry's expansion", len(inner))
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

// A flag argument that holds an unflagged `$y[1]` is cut at that `]` while
// its own `[` is open, so the `[` after it is pattern text, not a later
// subscript: the flag-pattern adapter keeps `$y[1][2]` as one pattern and
// no expansion gains later subscripts (#252).
func TestSecondSubscriptNotOpenedAfterFlagPatternCut(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$y[1][2]]}\n",
		"print \"${x[(r)$y[1][2]]}\"\n",
		"z=${x[(r)$y[1][2]]}\n",
	} {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src), "second-subscript.zsh")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if seconds := file.SecondSubscripts(); len(seconds) != 0 {
				t.Fatalf("SecondSubscripts() = %+v, want none", seconds)
			}
			if got := wordSource(src, soleFlagsArithm(t, file).X); got != "$y[1][2]" {
				t.Fatalf("pattern = %q, want $y[1][2]", got)
			}
		})
	}
}

func TestSecondSubscriptRejectsInvalidSources(t *testing.T) {
	// An unterminated later subscript is reported at its own `[`, which the
	// parser now reads as a subscript of the expansion (#252).
	tests := []struct {
		fixture string
		text    string
		col     uint
	}{
		{"testdata/invalid-215-unterminated-second-subscript.txt", "reached `}` without matching `[` with `]`", 13},
		{"testdata/invalid-215-double-close-before-second-subscript.txt", "not a valid parameter expansion operator: `]`", 13},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			src, err := os.ReadFile(tt.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-215.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			assertSecondSubscriptError(t, err, tt.text, 2, tt.col)
		})
	}
}

func assertSecondSubscriptError(t *testing.T, err error, text string, line, col uint) {
	t.Helper()
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v (%T), want a syntax.ParseError", err, err)
	}
	if parseErr.Text != text {
		t.Errorf("error text = %q, want %q", parseErr.Text, text)
	}
	if parseErr.Pos.Line() != line || parseErr.Pos.Col() != col {
		t.Errorf("error position = %d:%d, want %d:%d", parseErr.Pos.Line(), parseErr.Pos.Col(), line, col)
	}
}

// A later, unrelated error keeps its true position after the masked retry.
func TestSecondSubscriptPreservesLaterErrorPosition(t *testing.T) {
	src := "print ${a[b][1,50]}\nif true; then\n"
	_, err := Parse(strings.NewReader(src), "second-subscript.zsh")
	if err == nil {
		t.Fatal("Parse() accepted an unterminated if")
	}
	assertSecondSubscriptError(t, err, "`if` statement must end with `fi`", 2, 1)
}

// The printed tree keeps every subscript: later ones are typed children of
// the expansion, which the printer writes after the first (#252).
func TestSecondSubscriptPrintsEverySubscript(t *testing.T) {
	file, err := Parse(strings.NewReader("print ${a[b][1,50]:-x}\n"), "second-subscript.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print: %v", err)
	}
	if got := rendered.String(); got != "print ${a[b][1,50]:-x}\n" {
		t.Fatalf("printed = %q, want ${a[b][1,50]:-x}", got)
	}
}

// Every later subscript is reached once by a walk of the tree itself, so a
// consumer of File.AST needs no separate pass over SecondSubscripts (#252).
func TestSecondSubscriptsAreTreeChildren(t *testing.T) {
	src := "print ${a[1][${b[2][3]}][4]} \"${c[d][$(e)]}\"\n"
	file, err := Parse(strings.NewReader(src), "second-subscript.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	visits := make(map[syntax.Node]int)
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if node != nil {
			visits[node]++
		}
		return true
	})
	seconds := file.SecondSubscripts()
	if len(seconds) != 3 {
		t.Fatalf("SecondSubscripts() = %d entries, want 3", len(seconds))
	}
	for _, second := range seconds {
		if visits[second.Expansion] != 1 {
			t.Errorf("expansion %s visited %d times, want 1", nodeText(src, second.Expansion), visits[second.Expansion])
		}
		for _, subscript := range second.Subscripts {
			if visits[subscript] != 1 {
				t.Errorf("subscript %s visited %d times, want 1", nodeText(src, subscript), visits[subscript])
			}
		}
	}
}
