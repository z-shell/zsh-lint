package syntax

import (
	"strings"
	"testing"
)

// A word beginning with `#` starts a comment (zshmisc COMMENTS), while a `#`
// glued to an expansion or quote continues that word (#660). Inside nested
// command lists, case arms and array elements, every word start must keep its comment reading and every
// glued `#` must stay in its word. Each source passes `zsh -f -n`.
func TestZshHashWordInNestedLists(t *testing.T) {
	tests := []struct {
		src      string
		comments []string
		words    []string // printed words that must appear in the tree
	}{
		{"x=$(#c\n:)\n", []string{"c"}, nil},
		{"( #c\n: )\n", []string{"c"}, nil},
		{"( print ${x}; #c\n)\n", []string{"c"}, []string{"${x}"}},
		{"( print ${x}$(#c\n:) )\n", []string{"c"}, nil},
		{"x=`#c\n:`\n", []string{"c"}, nil},
		{"x=$( print ${x}#y ; #c\n)\n", []string{"c"}, []string{"${x}#y"}},
		{"( print ${x}#y\n#c\n)\n", []string{"c"}, []string{"${x}#y"}},
		{"( print `echo a`#c )\n", nil, []string{"`echo a`#c"}},
		{"( print \"${x}\"#c )\n", nil, []string{"\"${x}\"#c"}},
		{"( print $(echo a)#y )\n", nil, []string{"$(echo a)#y"}},
		{"( print ${x}#$y )\n", nil, []string{"${x}#$y"}},
		// Array elements are read the same way (#587).
		{"a=( ${x}#y $x#z \"q\"#r )\n", nil, []string{"${x}#y", "$x#z", "\"q\"#r"}},
		{"a=(x(b)#c)\n", nil, []string{"x(b)#c"}},
		{"a=( #c\nx )\n", []string{"c"}, []string{"x"}},
		{"a=( x #c\n)\n", []string{"c"}, []string{"x"}},
		// So are case arm bodies.
		{"case a in (x) print ${x}#c \"q\"#d ;; esac\n", nil, []string{"${x}#c", "\"q\"#d"}},
		{"case a in (x) #c\n: ;; esac\n", []string{"c"}, nil},
	}
	for _, tc := range tests {
		f, err := NewParser(Variant(LangZsh), KeepComments(true)).Parse(strings.NewReader(tc.src), "")
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		var comments, words []string
		Walk(f, func(node Node) bool {
			switch node := node.(type) {
			case *Comment:
				comments = append(comments, node.Text)
			case *Word:
				words = append(words, tc.src[node.Pos().Offset():node.End().Offset()])
			}
			return true
		})
		if strings.Join(comments, "|") != strings.Join(tc.comments, "|") {
			t.Errorf("%q: comments %q, want %q", tc.src, comments, tc.comments)
		}
		for _, want := range tc.words {
			found := false
			for _, got := range words {
				if got == want {
					found = true
				}
			}
			if !found {
				t.Errorf("%q: no word %q in %q", tc.src, want, words)
			}
		}
	}
}

// The word continuation is Zsh's reading; other dialects keep upstream's.
func TestHashWordInSubshellDialectGate(t *testing.T) {
	src := "( echo ${x}#y )\n"
	for _, lang := range []LangVariant{LangBash, LangMirBSDKorn} {
		f, err := NewParser(Variant(lang), KeepComments(true)).Parse(strings.NewReader(src), "")
		if err == nil {
			comments := 0
			Walk(f, func(node Node) bool {
				if _, ok := node.(*Comment); ok {
					comments++
				}
				return true
			})
			if comments == 0 {
				t.Errorf("%v: the `#` continued the word; this change must stay Zsh-only", lang)
			}
		}
	}
}
