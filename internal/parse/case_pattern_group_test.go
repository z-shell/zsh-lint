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
