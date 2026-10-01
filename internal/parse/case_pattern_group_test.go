package parse

import (
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// casePatterns prints the patterns of every case item in tree.
func casePatterns(t *testing.T, tree *syntax.File) [][]string {
	t.Helper()
	var items [][]string
	syntax.Walk(tree, func(node syntax.Node) bool {
		item, ok := node.(*syntax.CaseItem)
		if !ok {
			return true
		}
		var patterns []string
		for _, word := range item.Patterns {
			var rendered strings.Builder
			if err := syntax.NewPrinter().Print(&rendered, word); err != nil {
				t.Fatalf("print pattern: %v", err)
			}
			patterns = append(patterns, rendered.String())
		}
		items = append(items, patterns)
		return true
	})
	return items
}

// Zsh reads a case pattern that begins with `(` as one word. When the word's
// leading group is glued to more pattern text, the group is part of the
// pattern rather than the optional opener, and a leading `((` is the opener
// followed by a group (#452). The parser fork reads both, and a lone group
// followed by the command stays the opener.
func TestCasePatternGroup(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want []string
	}{
		{"case xy in\n  (x)y) print hit ;;\nesac", []string{"(x)y"}},
		{"case z in\n  ((x|y)|z) print hit ;;\nesac", []string{"(x|y)", "z"}},
		{"case x in\n  (x|y)) : ;;\nesac", []string{"(x|y)"}},
		{"case x in\n  ((x|y))) : ;;\nesac", []string{"((x|y))"}},
		{"case xy in (x)(y)) : ;; esac", []string{"(x)(y)"}},
		{"case xy in ((x)y) : ;; esac", []string{"(x)y"}},
		{"case x in (x)|y) : ;; esac", []string{"(x)", "y"}},
		{"case x in (x)y|z) : ;; esac", []string{"(x)y", "z"}},
		{"case x in (a|b)|(c)) : ;; esac", []string{"(a|b)", "(c)"}},
		{"case x in ($(print x)|y)) : ;; esac", []string{"($(print x)|y)"}},
		{"case x in (x) print hit ;; esac", []string{"x"}},
		{"case x in (x|y) : ;; esac", []string{"x", "y"}},
		{"case x in (x);; esac", []string{"x"}},
		{"case x in (x)\n  print ;; esac", []string{"x"}},
		{"case x in ((x|y)) : ;; esac", []string{"(x|y)"}},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		got := casePatterns(t, file.AST())
		if len(got) != 1 || strings.Join(got[0], "|") != strings.Join(tt.want, "|") || len(got[0]) != len(tt.want) {
			t.Errorf("%q: patterns = %q, want %q", tt.src, got, tt.want)
		}
	}
}

// Native-invalid shapes stay parse errors: an extra group closer, a group
// followed by more text with no closer, and an unclosed group.
func TestCasePatternGroupInvalid(t *testing.T) {
	for _, src := range []string{
		"case x in\n  (x|y))) : ;;\nesac",
		"case x in (x)y print;; esac",
		"case x in ((x|y) print;; esac",
		"case x in (x)print hit;; esac",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	src, err := os.ReadFile("testdata/invalid-452-group-pattern-without-closer.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(strings.NewReader(string(src)), "invalid-452.zsh"); err == nil {
		t.Fatal("Parse() accepted a group pattern with no closer")
	}
}

// Zsh reads a case item that begins with `(` as one word, so a `#` glued to
// that opener or to a `|` inside it is pattern text, as in the globbing flag
// `(#i)`, not the start of a comment (#482). A `#` glued to the `)` of a
// leading group is the glob operator `#`.
func TestCasePatternHashInOpenedGroup(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want []string
	}{
		{"case x in\n  (#i)x) print hit ;;\nesac", []string{"(#i)x"}},
		{"case x in\n  (#b)x) print hit ;;\nesac", []string{"(#b)x"}},
		{"case x in\n  (#b)(--adj)) print hit ;;\nesac", []string{"(#b)(--adj)"}},
		{"case x in\n  (#b)(--adj)(=(-|+|)[0-9]#|)) print hit ;;\nesac", []string{"(#b)(--adj)(=(-|+|)[0-9]#|)"}},
		{"case x in (#i)x|y) : ;; esac", []string{"(#i)x", "y"}},
		{"case x in (#i)x | y) : ;; esac", []string{"(#i)x", "y"}},
		{"case x in (#i)(x)) : ;; esac", []string{"(#i)(x)"}},
		{"case x in (#i)) : ;; esac", []string{"(#i)"}},
		{"case x in (#)) : ;; esac", []string{"(#)"}},
		{"case x in (#) : ;; esac", []string{"#"}},
		{"case x in (#)x) : ;; esac", []string{"(#)x"}},
		{"case x in (##)) : ;; esac", []string{"(##)"}},
		{"case x in (#ia2)x) : ;; esac", []string{"(#ia2)x"}},
		{"case x in (#s)x(#e)) : ;; esac", []string{"(#s)x(#e)"}},
		{"case x in (a|#i)x) : ;; esac", []string{"(a|#i)x"}},
		{"case x in (a|#i)) : ;; esac", []string{"(a|#i)"}},
		{"case x in (a|#i)x|y) : ;; esac", []string{"(a|#i)x", "y"}},
		{"case x in (a||#b)) : ;; esac", []string{"(a||#b)"}},
		{"case x in (#i)(a|#b)x) : ;; esac", []string{"(#i)(a|#b)x"}},
		{"case x in (x)#) : ;; esac", []string{"(x)#"}},
		{"case x in (x)##) : ;; esac", []string{"(x)##"}},
		{"case x in (x)#|y) : ;; esac", []string{"(x)#", "y"}},
		{"case x in (#i)x|(#i)y) : ;; esac", []string{"(#i)x", "(#i)y"}},
		{"case x in ($(print a)|#i)) : ;; esac", []string{"($(print a)|#i)"}},
		{"case x in ($(print a)|#i)x) : ;; esac", []string{"($(print a)|#i)x"}},
		{"case x in (#i)a) #c\n : ;; esac", []string{"(#i)a"}},
		{"case x in (a)#b) : ;; esac", []string{"(a)#b"}},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		got := casePatterns(t, file.AST())
		if len(got) != 1 || strings.Join(got[0], "|") != strings.Join(tt.want, "|") || len(got[0]) != len(tt.want) {
			t.Errorf("%q: patterns = %q, want %q", tt.src, got, tt.want)
		}
	}
}

// A `#` that starts a token in a case item is still a comment: after a `|`
// when the item has no opener or its leading group has closed, after the
// closing `)`, inside a command substitution in the pattern, and anywhere
// after the case clause. Zsh rejects each erroring shape here, and reads
// the pattern-closing one as a comment after the pattern (#482).
func TestCasePatternHashStartsComment(t *testing.T) {
	for _, src := range []string{
		"case x in\n  x|#i) : ;;\nesac",
		"case x in\n  x|#i)y) : ;;\nesac",
		"case x in\n  (x)|#i) : ;;\nesac",
		"case x in\n  (x)y|#i) : ;;\nesac",
		"case x in\n  (#i)x|#j) : ;;\nesac",
		"case x in\n  (a|$(print #c))x) : ;;\nesac",
		"case x in\n  (a|$(print x|#c))x) : ;;\nesac",
		"case x in\n  (#i)x print a ;;\nesac",
		"case x in (a) : ;; esac |#c",
		"case x in (#i)a) : ;; esac |#c",
		"case x in (a) : esac |#c",
		"print $(case x in (a) : ;; esac) |#c",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	const after = "case x in\n  (#i)x)#) print a ;;\nesac\n"
	file, err := Parse(strings.NewReader(after), "t.zsh")
	if err != nil {
		t.Fatalf("%q: %v", after, err)
	}
	if got := casePatterns(t, file.AST()); len(got) != 1 || strings.Join(got[0], "|") != "(#i)x" {
		t.Errorf("%q: patterns = %q, want [[(#i)x]]", after, got)
	}
	var comments []string
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if c, ok := node.(*syntax.Comment); ok {
			comments = append(comments, c.Text)
		}
		return true
	})
	if strings.Join(comments, "|") != ") print a ;;" {
		t.Errorf("%q: comments = %q, want [\") print a ;;\"]", after, comments)
	}
}

// A syntax error after a case arm whose pattern holds a group keeps its
// position.
func TestCasePatternGroupPreservesLaterErrorPosition(t *testing.T) {
	const src = "case x in\n  (x|y)) : ;;\nesac\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a parse error", err)
	}
	if parseErr.Pos.Line() != 4 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 4:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

// Zsh reads a case item that begins with `(` as one word up to the matching
// `)`, so a blank inside a bracket expression of that word is part of the
// bracket expression, not a word break (#483).
func TestCasePatternBracketBlank(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want []string
	}{
		{"case x in\n  (a[ b]*) print a ;;\nesac", []string{"a[ b]*"}},
		{"case x in\n  (include[ $TAB]*) print a ;;\nesac", []string{"include[ $TAB]*"}},
		{"case x in\n  ([ ]) print a ;;\nesac", []string{"[ ]"}},
		{"case x in\n  (a|[ ]) print a ;;\nesac", []string{"a", "[ ]"}},
		{"case x in\n  ([ ]|a) print a ;;\nesac", []string{"[ ]", "a"}},
		{"case x in\n  (a[\tb]*) print a ;;\nesac", []string{"a[\tb]*"}},
		{"case x in\n  (a[ ]b[ ]c) print a ;;\nesac", []string{"a[ ]b[ ]c"}},
		{"case x in\n  ([[:space:] ]) print a ;;\nesac", []string{"[[:space:] ]"}},
		{"case x in\n  ([^ ]) print a ;;\nesac", []string{"[^ ]"}},
		{"case x in\n  ([] ]) print a ;;\nesac", []string{"[] ]"}},
		{"case x in\n  (x[ ]y|z) print a ;;\nesac", []string{"x[ ]y", "z"}},
		{"case x in\n  (a[$x b]) print a ;;\nesac", []string{"a[$x b]"}},
		{"case x in\n  (a[\"x\" b]) print a ;;\nesac", []string{"a[\"x\" b]"}},
		{"case x in\n  (a[$(print x) b]) print a ;;\nesac", []string{"a[$(print x) b]"}},
		{"case x in\n  (a[${x:- y} b]) print a ;;\nesac", []string{"a[${x:- y} b]"}},
		{"case x in\n  (a[$x b]|c[$y d]) print a ;;\nesac", []string{"a[$x b]", "c[$y d]"}},
		{"case x in\n  (\"a\"[ b]) print a ;;\nesac", []string{"\"a\"[ b]"}},
		{"case x in\n  (a) print [ b] ;;\nesac", []string{"a"}},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		got := casePatterns(t, file.AST())
		if len(got) != 1 || strings.Join(got[0], "\x00") != strings.Join(tt.want, "\x00") || len(got[0]) != len(tt.want) {
			t.Errorf("%q: patterns = %q, want %q", tt.src, got, tt.want)
		}
	}
}

// A blank keeps its pattern-text reading only inside a bracket expression of
// the group a case item's `(` opens (#483). Without that `(`, or in text glued
// to a leading group after it has closed, the blank ends the word and Zsh
// rejects the item; `;`, `&`, `<` and `)` end the word even inside the
// brackets.
func TestCasePatternBracketBlankWithoutOpener(t *testing.T) {
	for _, src := range []string{
		"case x in\n  a[ b]*) print a ;;\nesac",
		"case x in\n  x|a[ b]) print a ;;\nesac",
		"case x in\n  (a[ ;]) print a ;;\nesac",
		"case x in\n  (a[ &]) print a ;;\nesac",
		"case x in\n  (a[ <]) print a ;;\nesac",
		"case x in\n  (x)y[ z]) print a ;;\nesac",
		"case x in\n  (x)[ ]) print a ;;\nesac",
		"case x in\n  (a[$x;b]) print a ;;\nesac",
		"case x in\n  (a[$x) b]) print a ;;\nesac",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
}

// A `((` glued to text before it in a case pattern opens a glob group
// holding a nested group, as in `x((b)c)`, so the pattern is one word
// (#481). Missing the pattern's own `)`, the item stays a parse error.
func TestCasePatternGluedNestedGroup(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want []string
	}{
		{"case x in\n  x((b)c)) print a ;;\nesac", []string{"x((b)c)"}},
		{"case x in\n  y|x((b)c)) print a ;;\nesac", []string{"y", "x((b)c)"}},
		{"case x in\n  (x((b)c)|y) print a ;;\nesac", []string{"x((b)c)", "y"}},
		{"case x in\n  --((a)|b)) print a ;;\nesac", []string{"--((a)|b)"}},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		got := casePatterns(t, file.AST())
		if len(got) != 1 || strings.Join(got[0], "\x00") != strings.Join(tt.want, "\x00") || len(got[0]) != len(tt.want) {
			t.Errorf("%q: patterns = %q, want %q", tt.src, got, tt.want)
		}
	}
	src, err := os.ReadFile("testdata/invalid-481-glued-group-pattern-without-closer.txt")
	if err != nil {
		t.Fatal(err)
	}
	var parseErr syntax.ParseError
	if _, err := Parse(strings.NewReader(string(src)), "invalid-481.zsh"); !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
}

// Native Zsh accepts a case pattern with an empty alternative (#396). The
// empty alternative matches the empty string.
func TestCasePatternEmptyAlternative(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want []string
	}{
		{"case x in\n  (|a) print hit ;;\nesac", []string{"", "a"}},
		{"case x in\n  (a|) print hit ;;\nesac", []string{"a", ""}},
		{"case x in\n  |a) print hit ;;\nesac", []string{"", "a"}},
		{"case x in\n  a|) print hit ;;\nesac", []string{"a", ""}},
		{"case x in\n  |a|) print hit ;;\nesac", []string{"", "a", ""}},
		{"case x in\n  (a||b) print hit ;;\nesac", []string{"a", "", "b"}},
		{"case x in\n  (|) print hit ;;\nesac", []string{"", ""}},
		{"case x in\n  ||) print hit ;;\nesac", []string{"", "", ""}},
		{"case x in\n  (|a|) print hit ;;\nesac", []string{"", "a", ""}},
		{"case x in\n  (|https|git|http|ftp|ftps|rsync|ssh) print hit ;;\nesac", []string{"", "https", "git", "http", "ftp", "ftps", "rsync", "ssh"}},
		{"case x in\n  (a||) print hit ;;\nesac", []string{"a", "", ""}},
		{"case x in\n  |) print hit ;;\nesac", []string{"", ""}},
		{"case x in\n  (||) print hit ;;\nesac", []string{"", "", ""}},
		{"case x in\n  a||b) print hit ;;\nesac", []string{"a", "", "b"}},
		{"case x in\n  |a|b) print hit ;;\nesac", []string{"", "a", "b"}},
		{"case x in\n  (a|b|) print hit ;;\nesac", []string{"a", "b", ""}},
		{"case x in (|a) print hit ;; esac", []string{"", "a"}},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		got := casePatterns(t, file.AST())
		if len(got) != 1 || strings.Join(got[0], "|") != strings.Join(tt.want, "|") || len(got[0]) != len(tt.want) {
			t.Errorf("%q: patterns = %q, want %q", tt.src, got, tt.want)
		}
	}
}

// Native-invalid case patterns stay parse errors: empty parens, unclosed
// pattern, and lone right paren.
func TestCasePatternEmptyAlternativeInvalid(t *testing.T) {
	for _, src := range []string{
		"case x in () print hit ;; esac",
		"case x in ( ) print hit ;; esac",
		"case x in ) print hit ;; esac",
		"case x in (|a print hit ;;\nesac",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	for _, file := range []string{
		"testdata/invalid-396-empty-parens-case-pattern.txt",
		"testdata/invalid-396-unclosed-case-pattern.txt",
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(strings.NewReader(string(src)), file); err == nil {
			t.Fatalf("Parse(%s) accepted invalid source", file)
		}
	}
}
