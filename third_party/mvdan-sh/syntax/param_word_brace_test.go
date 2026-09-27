package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #518. Outside double quotes Zsh nests braces in the word of a
// parameter expansion (bct in gettokstr, Src/lex.c): in `${x:-{a}b}` the
// first `}` closes the `{`, and an unclosed `{` leaves the expansion open.
// Inside double quotes, and in a here-document body, the first `}` closes
// it. Native verdicts use newline-terminated files and zsh -f -n.
func TestZshParamWordBracesAccept(t *testing.T) {
	for _, tc := range []struct{ src, word string }{
		{"print ${x:-{}}", "${x:-{}}"},
		{"print ${x:-a{b}c}", "${x:-a{b}c}"},
		{"print ${x:-{a}b}", "${x:-{a}b}"},
		{"print ${x:-{{}}}", "${x:-{{}}}"},
		{"print ${x/a/{}}", "${x/a/{}}"},
		{"print ${x/{}/b}", "${x/{}/b}"},
		{"print ${x#{}}", "${x#{}}"},
		{"print ${x%{}}", "${x%{}}"},
		{"print ${x:={}}", "${x:={}}"},
		{"print ${x:+{}}", "${x:+{}}"},
		{"print ${x:?{}}", "${x:?{}}"},
		{"x=${x:-{}}", "${x:-{}}"},
		{"[[ ${x:-{}} == a ]]", "${x:-{}}"},
		{"f() { print ${x:-{}}; }", "${x:-{}}"},
		{"print ${x:-{} ; print y }", "${x:-{} ; print y }"},
		// A nested expansion keeps its own count, and the enclosing count
		// resumes at the `}` after it.
		{"print ${x:-${y:-{}}}", "${x:-${y:-{}}}"},
		{"print ${x:-${y}{}}", "${x:-${y}{}}"},
		{"print ${x:-{$y}}", "${x:-{$y}}"},
		{"print ${x:-{${y}}}", "${x:-{${y}}}"},
		{"print ${x:-{${y:1:2}}}", "${x:-{${y:1:2}}}"},
		{"print ${x:-{${y}a}b}", "${x:-{${y}a}b}"},
		// An inner expansion with an operator word closes through a
		// different path; the outer count must still resume after it.
		{"print ${x:-{${y:-a}}}", "${x:-{${y:-a}}}"},
		{"print ${x:-{${y#a}}}", "${x:-{${y#a}}}"},
		{"print ${x:-a${y:-{}}b}", "${x:-a${y:-{}}b}"},
		// Quoted and escaped braces are text and are not counted.
		{"print ${x:-\\{}", "${x:-\\{}"},
		{"print ${x:-'{'}", "${x:-'{'}"},
		{"print ${x:-\"{\"}", "${x:-\"{\"}"},
		// Inside double quotes, or a here-document body, the first `}` closes.
		{"print \"${x:-{}\"", "${x:-{}"},
		{"x=\"${${x:-{}}\"", "${${x:-{}}"},
		{"x=\"${${x:-{a\\}}#a}\"", "${${x:-{a\\}}#a}"},
		{"cat <<EOF\n${x:-{}\nEOF", "${x:-{}"},
		{"cat <<-EOF\n	${x:-{}\n	EOF", "${x:-{}"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var outer *syntax.ParamExp
			syntax.Walk(f, func(n syntax.Node) bool {
				if pe, ok := n.(*syntax.ParamExp); ok && outer == nil {
					outer = pe
				}
				return true
			})
			if outer == nil {
				t.Fatal("no parameter expansion")
			}
			if got := tc.src[outer.Pos().Offset():outer.End().Offset()]; got != tc.word {
				t.Fatalf("expansion = %q, want %q", got, tc.word)
			}
		})
	}
}

func TestZshParamWordBracesReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"print ${x:-{}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x:-a{b}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x:-{a}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x:-{}a", "1:7: reached EOF without matching `${` with `}`"},
		{"x=${x:-{}", "1:3: reached EOF without matching `${` with `}`"},
		{"print ${x/a/{}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x/{/b}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x#{}", "1:7: reached EOF without matching `${` with `}`"},
		{"print ${x:-{$y}", "1:7: reached EOF without matching `${` with `}`"},
		// A nested expansion counts its own braces from zero: the inner
		// `{}` closes, and the outer expansion is left open.
		{"print ${x:-${y:-{}}", "1:7: reached EOF without matching `${` with `}`"},
		// A nested expansion inherits double quotes, where a `'` is text
		// (#400): this `}` closes the inner expansion and the `'` after it
		// is not an operator. Zsh reports a bad substitution.
		{"print \"${${x:-'}'}}\"", "1:17: not a valid parameter expansion operator: `'`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// Other dialects close the expansion at the first `}`, as before.
func TestZshParamWordBracesDialectGate(t *testing.T) {
	for _, lang := range []syntax.LangVariant{syntax.LangBash, syntax.LangMirBSDKorn} {
		f, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader("echo ${x:-{}\n"), "")
		if err != nil {
			t.Fatalf("%s rejected `echo ${x:-{}`: %v", lang, err)
		}
		var pe *syntax.ParamExp
		syntax.Walk(f, func(n syntax.Node) bool {
			if p, ok := n.(*syntax.ParamExp); ok && pe == nil {
				pe = p
			}
			return true
		})
		if pe == nil || pe.End().Offset() != uint(len("echo ${x:-{}")) {
			t.Fatalf("%s: expansion does not end at the first `}`", lang)
		}
	}
}
