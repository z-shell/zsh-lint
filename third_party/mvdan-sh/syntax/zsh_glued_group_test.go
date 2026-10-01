package syntax

import (
	"strings"
	"testing"
)

// zshGluedGroupWords parses src as Zsh and returns the words of every simple
// command and every case pattern, each word printed and wrapped in <...>,
// one string per command or case item.
func zshGluedGroupWords(t *testing.T, src string) ([]string, error) {
	t.Helper()
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\n"), "")
	if err != nil {
		return nil, err
	}
	pw := func(w *Word) string {
		var sb strings.Builder
		if err := NewPrinter().Print(&sb, w); err != nil {
			t.Fatal(err)
		}
		return "<" + sb.String() + ">"
	}
	var out []string
	Walk(f, func(node Node) bool {
		var words []string
		switch n := node.(type) {
		case *CallExpr:
			for _, a := range n.Assigns {
				if a.Value != nil {
					words = append(words, a.Name.Value+"="+pw(a.Value))
				}
				if a.Array != nil {
					for _, el := range a.Array.Elems {
						words = append(words, a.Name.Value+"[]"+pw(el.Value))
					}
				}
			}
			for _, w := range n.Args {
				words = append(words, pw(w))
			}
		case *CaseItem:
			for _, w := range n.Patterns {
				words = append(words, pw(w))
			}
		case *Redirect:
			words = append(words, ">"+pw(n.Word))
		case *WordIter:
			for _, w := range n.Items {
				words = append(words, "in"+pw(w))
			}
		default:
			return true
		}
		out = append(out, strings.Join(words, ""))
		return true
	})
	return out, nil
}

// Zsh reads a `((` that continues a word as a glob group opened inside
// another one, not as the start of an arithmetic command, so `x((b)c)` is
// one word (zsh-lint #481). Each source passes `zsh -f -n`.
func TestZshGluedGroup(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src  string
		want string // results of zshGluedGroupWords joined by "/"
	}{
		{"print x((b)c)", "<print><x((b)c)>"},
		{"print x((b))", "<print><x((b))>"},
		{"print x(((b)))", "<print><x(((b)))>"},
		{"print x((b)c)d", "<print><x((b)c)d>"},
		{"print x((b|c)d)", "<print><x((b|c)d)>"},
		{"print x((b)c) y", "<print><x((b)c)><y>"},
		{"print x((1+2))", "<print><x((1+2))>"},
		{"print x((#i)b)", "<print><x((#i)b)>"},
		{"print x(($y)c)", "<print><x(($y)c)>"},
		{"print x((\"b\")c)", "<print><x((\"b\")c)>"},
		{"print x(($(print b))c)", "<print><x(($(print b))c)>"},
		{"print x((b)c)((d)e)", "<print><x((b)c)((d)e)>"},
		{"print $x((b)c)", "<print><$x((b)c)>"},
		{"print ${x}((b)c)", "<print><${x}((b)c)>"},
		{"print \"a\"((b)c)", "<print><\"a\"((b)c)>"},
		{"print 'a'((b)c)", "<print><'a'((b)c)>"},
		{"x((b)c) arg", "<x((b)c)><arg>"},
		{"a=x((b)c)", "a=<x((b)c)>"},
		{"a=(y x((b)c))", "a[]<y>a[]<x((b)c)>"},
		{"print a > x((b)c)", "<print><a>/>" + "<x((b)c)>"},
		{"print x((b)c)>y", "<print><x((b)c)>/><y>"},
		{"print x((b)c);:", "<print><x((b)c)>/<:>"},
		{"for i in x((b)c) y; do :; done", "in<x((b)c)>in<y>/<:>"},
		{"case $x in x((b)c)) : ;; esac", "<x((b)c)>/<:>"},
		{"case $x in x((b))) : ;; esac", "<x((b))>/<:>"},
		{"case $x in x(((b)c)d)) : ;; esac", "<x(((b)c)d)>/<:>"},
		{"case $x in y|x((b)c)) : ;; esac", "<y><x((b)c)>/<:>"},
		{"case $x in x((b)c)|y) : ;; esac", "<x((b)c)><y>/<:>"},
		{"case $x in x((b)c)y) : ;; esac", "<x((b)c)y>/<:>"},
		{"case $x in --((a)|b)) : ;; esac", "<--((a)|b)>/<:>"},
		{"case $x in (x((b)c)) : ;; esac", "<x((b)c)>/<:>"},
	} {
		got, err := zshGluedGroupWords(t, tt.src)
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		if g := strings.Join(got, "/"); g != tt.want {
			t.Errorf("%q: words %q, want %q", tt.src, g, tt.want)
		}
	}
}

// The name after the `function` keyword is a word too, so a `((` that
// continues it opens a glob group (zsh-lint #481).
func TestZshGluedGroupFuncName(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src  string
		want string
	}{
		{"function x((b)c) { : }", "x((b)c)"},
		{"function $x((b)c) { : }", "$x((b)c)"},
		{"function \"x\"((b)c) { : }", "\"x\"((b)c)"},
	} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tt.src+"\n"), "")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		var names []string
		Walk(f, func(node Node) bool {
			if fd, ok := node.(*FuncDecl); ok {
				for _, w := range fd.Names {
					var sb strings.Builder
					if err := NewPrinter().Print(&sb, w); err != nil {
						t.Fatal(err)
					}
					names = append(names, sb.String())
				}
				if fd.Name != nil && len(fd.Names) == 0 {
					names = append(names, fd.Name.Value)
				}
			}
			return true
		})
		if g := strings.Join(names, "|"); g != tt.want {
			t.Errorf("%q: names %q, want %q", tt.src, g, tt.want)
		}
	}
}

// A `((` keeps its arithmetic reading where it starts a command, and the
// shapes Zsh rejects stay parse errors: a `((` closed at once, a `;` inside
// the group, and a case pattern whose group swallows the pattern's `)`
// (zsh-lint #481).
func TestZshGluedGroupKeepsOtherReadings(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"((x))",
		"x=1;((x))",
		"while ((x)); do :; done",
		"f() ((x))",
		"f()((x))",
		"print x$((1+2))",
	} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	for _, src := range []string{
		"print x(())",
		"print x((b);c)",
		"case $x in x((b)c) : ;; esac",
		"case $x in x(()c)) : ;; esac",
		"case $x in x((b;c)c)) : ;; esac",
	} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err == nil {
			t.Errorf("%q: parsed, want an error", src)
		}
	}
}
