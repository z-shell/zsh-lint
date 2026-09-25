package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// zshmisc composes two productions: `for name ... [ in word ... ] term do list
// done` names more than one variable, and `for name ( word ... ) sublist` is
// the alternate form. The front end handled each alone but not together, so
// `for a b (1 2) { : }` kept the header error (#324).
//
// The names bind in tuples: `for a b (1 2 3 4)` runs with a=1 b=2, then a=3
// b=4. The upstream WordIter holds one name, so the extra names are masked and
// the tree is the same shape the `for a b in 1 2; do ... done` form gives.
func TestMultiNameParenFor(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantItems []string
	}{
		{"two names brace body", "for a b (1 2) { : }\n", []string{"1", "2"}},
		{"two names four items", "for a b (1 2 3 4) { print $a $b }\n", []string{"1", "2", "3", "4"}},
		{"three names", "for a b c (1 2 3) { : }\n", []string{"1", "2", "3"}},
		{"sublist body", "for a b (1 2) :\n", []string{"1", "2"}},
		{"single name unchanged", "for a (1 2) { : }\n", []string{"1", "2"}},
		{"nested loops", "for a b (1 2) { for c d (3 4) { : } }\n", []string{"1", "2", "3", "4"}},
		{"quoted paren in word", "for a b ('x)' 2) { : }\n", []string{"'x)'", "2"}},
		{"command substitution", "for a b ($(print 1 2)) { : }\n", []string{"$(print 1 2)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			var items []string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				clause, ok := node.(*syntax.ForClause)
				if !ok {
					return true
				}
				iter, ok := clause.Loop.(*syntax.WordIter)
				if !ok {
					t.Fatalf("loop is %T, want *syntax.WordIter", clause.Loop)
				}
				if iter.Name == nil {
					t.Fatal("WordIter.Name is nil, want the first loop name")
				}
				for _, item := range iter.Items {
					items = append(items, tc.src[item.Pos().Offset():item.End().Offset()])
				}
				return true
			})
			if strings.Join(items, " ") != strings.Join(tc.wantItems, " ") {
				t.Errorf("items = %q, want %q", items, tc.wantItems)
			}
		})
	}
}

// The masked names must not move anything: a diagnostic points at the original
// source, so items and body keep their real offsets.
func TestMultiNameParenForKeepsPositions(t *testing.T) {
	src := "for alpha beta (one two) { print body }\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}
	var checked bool
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		clause, ok := node.(*syntax.ForClause)
		if !ok {
			return true
		}
		iter := clause.Loop.(*syntax.WordIter)
		checked = true
		if got, want := int(iter.Name.ValuePos.Offset()), strings.Index(src, "alpha"); got != want {
			t.Errorf("name offset = %d, want %d", got, want)
		}
		if got, want := int(iter.Items[0].Pos().Offset()), strings.Index(src, "one"); got != want {
			t.Errorf("first item offset = %d, want %d", got, want)
		}
		if got, want := int(iter.Items[1].Pos().Offset()), strings.Index(src, "two"); got != want {
			t.Errorf("second item offset = %d, want %d", got, want)
		}
		if len(clause.Do) == 0 {
			t.Fatal("loop body is empty")
		}
		if got, want := int(clause.Do[0].Pos().Offset()), strings.Index(src, "print"); got != want {
			t.Errorf("body offset = %d, want %d", got, want)
		}
		return false
	})
	if !checked {
		t.Fatal("no ForClause in the tree")
	}
}

// The extra-name scan runs before the `(`, so a word that ends the name list
// must not be eaten. Each source here is valid Zsh meaning something else.
func TestMultiNameParenForLeavesOtherHeaders(t *testing.T) {
	cases := []struct{ name, src string }{
		{"arithmetic for", "for ((i = 0; i < 2; i++)) { : }\n"},
		{"in before paren", "for a in (1 2); do :; done\n"},
		{"select paren form", "select a (1 2) { : }\n"},
		{"plain in form", "for a b in 1 2; do :; done\n"},
		{"glob word list", "for a b (*.zsh) { : }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// Native Zsh rejects these, so the adapter must not rescue them.
func TestMultiNameParenForRejectsInvalid(t *testing.T) {
	for _, name := range []string{
		"testdata/invalid-324-unclosed-word-list.txt",
		"testdata/invalid-324-in-before-paren-list.txt",
	} {
		t.Run(name, func(t *testing.T) {
			fixture, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, err := Parse(strings.NewReader(string(fixture)), "invalid-324.zsh"); err == nil {
				t.Fatalf("Parse() unexpectedly accepted %s", name)
			}
		})
	}
}
