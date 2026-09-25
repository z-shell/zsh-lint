package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Inside a Zsh glob group, a quoted `)` and the `)` that closes a command
// substitution do not end the group (#439). The parser fork reads the group
// as one literal, so the pattern word must keep the whole group.
func TestSubstitutionInPatternGroup(t *testing.T) {
	for _, tt := range []struct {
		src, want string
	}{
		{`[[ a == (a|$(print x)) ]]`, `(a|$(print x))`},
		{`[[ a == (a|"$(print x)") ]]`, `(a|"$(print x)")`},
		{`[[ a == (a|"$(print ")")") ]]`, `(a|"$(print ")")")`},
		{`[[ a == (a|(b|"$(print ")")")) ]]`, `(a|(b|"$(print ")")"))`},
		{`[[ a == (a|(b|"${x:-$(print ")")}")) ]]`, `(a|(b|"${x:-$(print ")")}"))`},
		{"[[ a == (a|(b|\"`print \")\"`\")) ]]", "(a|(b|\"`print \")\"`\"))"},
		{`[[ a == (a|$(print $(print x))) ]]`, `(a|$(print $(print x)))`},
		{`[[ a == (a|$((1+2))) ]]`, `(a|$((1+2)))`},
		{`[[ a == (a|(b|$(print x))) ]]`, `(a|(b|$(print x)))`},
		{`[[ a == (a|(b|$(print ")"))) ]]`, `(a|(b|$(print ")")))`},
		{"[[ a == (a|(b|`print x`)) ]]", "(a|(b|`print x`))"},
		{`[[ a == (a|(b|$((1+2)))) ]]`, `(a|(b|$((1+2))))`},
		{`[[ a == (a|(b|"$((1+2))")) ]]`, `(a|(b|"$((1+2))"))`},
		{`[[ a == (a|(b|"$(( (1+2) ))")) ]]`, `(a|(b|"$(( (1+2) ))"))`},
		{`[[ a == (a|')') ]]`, `(a|')')`},
		{`[[ a == (a|\)) ]]`, `(a|\))`},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%s: %v", tt.src, err)
			continue
		}
		if got := renderedPatternWords(t, file.AST()); len(got) != 1 || got[0] != tt.want {
			t.Errorf("%s: pattern words = %q, want [%q]", tt.src, got, tt.want)
		}
	}
}

// A glob qualifier is read by the same group rule, so a `)` quoted in its
// code no longer ends it.
func TestQuotedCloserInGlobQualifier(t *testing.T) {
	for _, tt := range []struct {
		src, want string
	}{
		{`print *(e:'[[ $REPLY == *\) ]]':)`, `*(e:'[[ $REPLY == *\) ]]':)`},
		{`print *(e:'print )':N)`, `*(e:'print )':N)`},
		{`print (a|"x)")`, `(a|"x)")`},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%s: %v", tt.src, err)
			continue
		}
		call, ok := file.AST().Stmts[0].Cmd.(*syntax.CallExpr)
		if !ok || len(call.Args) != 2 {
			t.Errorf("%s: want a two-word command, got %#v", tt.src, file.AST().Stmts[0].Cmd)
			continue
		}
		var got strings.Builder
		if err := syntax.NewPrinter().Print(&got, call.Args[1]); err != nil {
			t.Fatal(err)
		}
		if got.String() != tt.want {
			t.Errorf("%s: word = %q, want %q", tt.src, got.String(), tt.want)
		}
	}
}

// An unterminated quote or substitution inside the group still reaches the end
// of input, and an unbalanced group inside a substitution is still reported.
func TestSubstitutionInPatternGroupInvalid(t *testing.T) {
	for _, src := range []string{
		`[[ a == (a|"$(print x)) ]]`,
		`[[ a == (a|$(print x) ]]`,
		`[[ a == (a|"x) ]]`,
		`[[ a == (a|'x) ]]`,
		`[[ a == (a|${x) ]]`,
		`[[ a == (a|(b|$(print x)) ]]`,
		`[[ a == (a|(b|$((1+2))) ]]`,
		`[[ a == (a|(b|"$((1+2)")) ]]`,
		": $(\n[[ a == (a|(b|\")\") ]]\n)",
		": <(\n[[ a == (a|(b|\")\") ]]\n)",
		": \"$(\n[[ a == (a|(b|\")\") ]]\n)\"",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	src, err := os.ReadFile("testdata/invalid-439-unbalanced-group-in-substitution.txt")
	if err != nil {
		t.Fatal(err)
	}
	var parseErr syntax.ParseError
	if _, err := Parse(bytes.NewReader(src), "invalid-439.zsh"); !errors.As(err, &parseErr) ||
		parseErr.Text != "unmatched `(` in conditional pattern" {
		t.Fatalf("Parse() error = %v, want the unmatched group", err)
	}
}
