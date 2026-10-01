package syntax

import (
	"strings"
	"testing"
	"time"
)

// zshCaseBracketOpen decides whether a blank in a Zsh case item's opened
// group is inside a bracket expression (zsh-lint #483). Each row is the
// literal text read before the blank.
func TestZshCaseBracketOpen(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		carried bool
		lit     string
		want    bool
	}{
		{false, "", false},
		{false, "a", false},
		{false, "a[", true},
		{false, "a[b", true},
		{false, "a[b]", false},
		{false, "a[b]c[", true},
		{false, "[]", true},
		{false, "[]]", false},
		{false, "[!]", true},
		{false, "[^]", true},
		{false, "[!]]", false},
		{false, "[!", true},
		{false, "[[:alpha:]", true},
		{false, "[[:alpha:]]", false},
		{false, "[[:alpha:", true},
		{false, `a\[`, false},
		{false, `a[\]`, true},
		{false, `a[\]]`, false},
		{true, "", true},
		{true, "b", true},
		{true, "b]", false},
		{true, "b]c[", true},
		{true, "]", false},
	} {
		if got := zshCaseBracketOpen(tt.carried, []byte(tt.lit)); got != tt.want {
			t.Errorf("zshCaseBracketOpen(%v, %q) = %v, want %v", tt.carried, tt.lit, got, tt.want)
		}
	}
}

// A blank inside a bracket expression of a Zsh case item's opened group is
// pattern text, also after an expansion or quote inside the brackets, while
// a blank after the group closes or outside the brackets ends the word
// (zsh-lint #483). The pattern words are compared through the printer.
func TestZshCaseBracketBlank(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src  string
		want string // patterns joined by "|", or "" for a parse error
	}{
		{"case x in (a[ b]*) :;; esac", "a[ b]*"},
		{"case x in (a[$x b]) :;; esac", "a[$x b]"},
		{"case x in (a[${x[1 + 2]} b]) :;; esac", "a[${x[1 + 2]} b]"},
		{"case x in (a[$((1 + 2)) b]) :;; esac", "a[$((1 + 2)) b]"},
		{"case x in ([!] ]) :;; esac", "[!] ]"},
		{"case x in ([^] ]) :;; esac", "[^] ]"},
		{"case x in (a[x] | b) :;; esac", "a[x]|b"},
		{"case x in (a[$x] | b) :;; esac", "a[$x]|b"},
		{"case x in (a[x| b) :;; esac", "a[x|b"},
		{"case x in (a\\[ | b) :;; esac", "a\\[|b"},
		{"case x in (a[x b) :;; esac", "a[x b"},
		{"case x in (a[ (b) ]) :;; esac", "a[ (b) ]"},
		{"case x in (a[x (b) c]) :;; esac", "a[x (b) c]"},
		{"case x in (a[$x] b) :;; esac", ""},
		{"case x in (a[$x)y b) :;; esac", ""},
		{"case x in (x)y[ z]) :;; esac", ""},
		{"case x in a[ b]) :;; esac", ""},
		{"case x in (a[ ;]) :;; esac", ""},
	} {
		p := NewParser(Variant(LangZsh))
		f, err := p.Parse(strings.NewReader(tt.src+"\n"), "")
		if tt.want == "" {
			if err == nil {
				t.Errorf("%q: parsed, want an error", tt.src)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		var pats []string
		Walk(f, func(node Node) bool {
			if ci, ok := node.(*CaseItem); ok {
				for _, w := range ci.Patterns {
					var sb strings.Builder
					if err := NewPrinter().Print(&sb, w); err != nil {
						t.Fatal(err)
					}
					pats = append(pats, sb.String())
				}
			}
			return true
		})
		if got := strings.Join(pats, "|"); got != tt.want {
			t.Errorf("%q: patterns %q, want %q", tt.src, got, tt.want)
		}
	}
}

// The bracket reading ends with the case item's pattern list: a blank in the
// body, in a command substitution inside the pattern, or outside any case
// item still separates the words of a command, and a `|` in the pattern list
// strips the blank after it (zsh-lint #483).
func TestZshCaseBracketBlankScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src  string
		want []string // each command's words, each word in <...>
	}{
		{"case x in (a[x b) print [ c] ;; esac", []string{"<print><[><c]>"}},
		{"case x in (a[ b]) print [ c] ;; esac", []string{"<print><[><c]>"}},
		{"case x in (a$(print [ c])) :;; esac", []string{"<print><[><c]>", "<:>"}},
		{"case x in (a[$(print [ c]) d]) :;; esac", []string{"<print><[><c]>", "<:>"}},
		{"case x in (a[$(print b c)]) :;; esac", []string{"<print><b><c>", "<:>"}},
		{"print a[ b; print a[$x b", []string{"<print><a[><b>", "<print><a[$x><b>"}},
	} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tt.src+"\n"), "")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		var calls []string
		Walk(f, func(node Node) bool {
			if ce, ok := node.(*CallExpr); ok {
				var args []string
				for _, w := range ce.Args {
					var sb strings.Builder
					if err := NewPrinter().Print(&sb, w); err != nil {
						t.Fatal(err)
					}
					args = append(args, "<"+sb.String()+">")
				}
				calls = append(calls, strings.Join(args, ""))
			}
			return true
		})
		if strings.Join(calls, "/") != strings.Join(tt.want, "/") {
			t.Errorf("%q: commands %q, want %q", tt.src, calls, tt.want)
		}
	}
	const src = "case x in (a[x| b) :;; esac\n"
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	Walk(f, func(node Node) bool {
		if ci, ok := node.(*CaseItem); ok {
			if len(ci.Patterns) != 2 || ci.Patterns[1].Lit() != "b" {
				t.Errorf("%q: second pattern %q, want \"b\"", src, ci.Patterns[len(ci.Patterns)-1].Lit())
			}
		}
		return true
	})
}

// The lexer stops skipping blanks only at a blank the literal reader then
// keeps, so no shape around the #483 bracket rule stalls it: a carriage
// return after an expansion in the brackets, and words outside any case item.
// Each source must finish parsing, with or without an error.
func TestZshCaseBracketBlankTerminates(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"case x in (a[$x\r b]) :;; esac",
		"case x in (a[ \r]) :;; esac",
		"print a[ b",
		"print a[b] c",
		"x=(a[1] b)",
		"case x in (a[$x b]) print a[ b ;; esac",
	} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\n"), "")
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: parse did not finish", src)
		}
	}
}
