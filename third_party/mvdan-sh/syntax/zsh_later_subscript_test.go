package syntax

import (
	"strings"
	"testing"
)

// laterSubscriptCase is one expansion whose subscripts after the first are
// typed children (zsh-lint #252): Zsh applies each one to the result of the
// subscript before it.
// https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts
type laterSubscriptCase struct {
	src   string
	param string
	first string
	later []string
	// printed is the canonical form when it differs from src, as the
	// printer writes for a first subscript too.
	printed string
}

var laterSubscriptCases = []laterSubscriptCase{
	{src: "print ${a[1][2]}", param: "a", first: "1", later: []string{"2"}},
	{src: "print ${a[1][2][3]}", param: "a", first: "1", later: []string{"2", "3"}},
	{src: "print ${a[b][1,50]}", param: "a", first: "b", later: []string{"1,50"}},
	{src: "print ${a[1,2][3,4][5,6]}", param: "a", first: "1,2", later: []string{"3,4", "5,6"}},
	{src: "print ${#a[b][1]}", param: "a", first: "b", later: []string{"1"}},
	{src: "print ${+a[b][1]}", param: "a", first: "b", later: []string{"1"}},
	{src: "print ${(f)a[b][1]}", param: "a", first: "b", later: []string{"1"}},
	{src: "print ${a[b][-1]:-none}", param: "a", first: "b", later: []string{"-1"}},
	{src: "print ${a[b][(r)x*]}", param: "a", first: "b", later: []string{"(r)x*"}},
	{src: "print ${a[(i)x][2]}", param: "a", first: "(i)x", later: []string{"2"}},
	{src: `print -r -- ${a[1][$(eval "$code")]}`, param: "a", first: "1", later: []string{`$(eval "$code")`}},
	{src: "print -r -- ${a[1][`eval x`]}", param: "a", first: "1", later: []string{"`eval x`"}, printed: "print -r -- ${a[1][$(eval x)]}"},
	{src: `print "${sice[ps-on-unload][1,50]}"`, param: "sice", first: "ps-on-unload", later: []string{"1,50"}, printed: `print "${sice[ps - on - unload][1,50]}"`},
	{src: "x=${a[1][2]}", param: "a", first: "1", later: []string{"2"}},
	{src: "argv[$argi]=(-n ${argv[$argi][2,-1]})", param: "argv", first: "$argi", later: []string{"2,-1"}},
}

func laterSubscriptText(src string, node Node) string {
	return src[node.Pos().Offset():node.End().Offset()]
}

func TestZshLaterSubscripts(t *testing.T) {
	t.Parallel()
	for _, tc := range laterSubscriptCases {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var pe *ParamExp
			Walk(f, func(node Node) bool {
				if exp, ok := node.(*ParamExp); ok && pe == nil && exp.Param != nil && exp.Param.Value == tc.param && !exp.Short {
					pe = exp
				}
				return true
			})
			if pe == nil {
				t.Fatalf("no ${%s...} expansion", tc.param)
			}
			if got := laterSubscriptText(tc.src, pe.Index); got != tc.first {
				t.Errorf("Index = %q, want %q", got, tc.first)
			}
			if len(pe.LaterIndexes) != len(tc.later) {
				t.Fatalf("LaterIndexes = %d, want %d", len(pe.LaterIndexes), len(tc.later))
			}
			for i, want := range tc.later {
				if got := laterSubscriptText(tc.src, pe.LaterIndexes[i]); got != want {
					t.Errorf("LaterIndexes[%d] = %q, want %q", i, got, want)
				}
			}
			// Walk reaches every later subscript exactly once, after the first.
			visits := make(map[Node]int)
			var order []Node
			Walk(f, func(node Node) bool {
				if node != nil {
					visits[node]++
					order = append(order, node)
				}
				return true
			})
			last := -1
			for i, index := range append([]ArithmExpr{pe.Index}, pe.LaterIndexes...) {
				if visits[index] != 1 {
					t.Errorf("subscript %d visited %d times, want 1", i, visits[index])
				}
				at := -1
				for j, node := range order {
					if node == index {
						at = j
					}
				}
				if at <= last {
					t.Errorf("subscript %d walked out of source order", i)
				}
				last = at
			}
			var printed strings.Builder
			if err := NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			want := tc.src
			if tc.printed != "" {
				want = tc.printed
			}
			if got := printed.String(); got != want+"\n" {
				t.Errorf("printed = %q, want %q", got, want+"\n")
			}
		})
	}
}

// A later subscript that holds an expansion with its own later subscripts
// keeps both expansions, each with its own children, reached once by Walk.
func TestZshNestedLaterSubscripts(t *testing.T) {
	t.Parallel()
	const src = "print ${a[1][${b[2][3]}]}\n"
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	var exps []*ParamExp
	Walk(f, func(node Node) bool {
		if exp, ok := node.(*ParamExp); ok {
			exps = append(exps, exp)
		}
		return true
	})
	if len(exps) != 2 || exps[0].Param.Value != "a" || exps[1].Param.Value != "b" {
		t.Fatalf("expansions = %v, want a then b once each", exps)
	}
	if len(exps[0].LaterIndexes) != 1 || laterSubscriptText(src, exps[0].LaterIndexes[0]) != "${b[2][3]}" {
		t.Errorf("outer LaterIndexes = %v, want ${b[2][3]}", exps[0].LaterIndexes)
	}
	if len(exps[1].LaterIndexes) != 1 || laterSubscriptText(src, exps[1].LaterIndexes[0]) != "3" {
		t.Errorf("inner LaterIndexes = %v, want 3", exps[1].LaterIndexes)
	}
}

// A reused parser forgets where the last parse cut a flag argument, so a
// `]` at the same offset of the next source still opens a later subscript.
func TestZshLaterSubscriptParserReuse(t *testing.T) {
	t.Parallel()
	parser := NewParser(Variant(LangZsh))
	if _, err := parser.Parse(strings.NewReader("print ${x[(r)$y[1][2]]}\n"), ""); err == nil {
		t.Fatal("the fork alone accepted a cut flag pattern")
	}
	f, err := parser.Parse(strings.NewReader("print ${x[(r)abcd][2]}\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	pe := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ParamExp)
	if len(pe.LaterIndexes) != 1 {
		t.Fatalf("LaterIndexes = %d, want 1", len(pe.LaterIndexes))
	}
}

// Only adjacent brackets open another subscript, other dialects keep the
// operator error, and an unterminated later subscript is reported at its
// own bracket.
func TestZshLaterSubscriptErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lang LangVariant
		src  string
		want string
	}{
		{LangZsh, "print ${a[1] [2]}\n", "1:13: not a valid parameter expansion operator: ` `"},
		{LangZsh, "print ${a[b]][1]}\n", "1:13: not a valid parameter expansion operator: `]`"},
		{LangZsh, "print ${a[b][1}\n", "1:13: reached `}` without matching `[` with `]`"},
		// A flag argument stopped at a `]` while a `[` of its own text is
		// open: Zsh does not close the subscript there, so the next `[` is
		// pattern text, which the front end's flag-pattern adapters read.
		{LangZsh, "print ${x[(r)$y[1][2]]}\n", "1:19: not a valid parameter expansion operator: `[`"},
		{LangZsh, "print \"${x[(r)a[b][2]]}\"\n", "1:19: not a valid parameter expansion operator: `[`"},
		{LangBash, "echo ${a[1][2]}\n", "1:12: not a valid parameter expansion operator: `[`"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			_, err := NewParser(Variant(tc.lang)).Parse(strings.NewReader(tc.src), "")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}
