package syntax

import (
	"strings"
	"testing"
	"time"
)

// Zsh lexes the group that opens a case item as one word, so a blank or
// newline anywhere inside it is pattern text (Src/lex.c gettokstr breaks at
// a blank only outside parentheses). When that group is the whole pattern,
// par_case in Src/parse.c strips the spaces and tabs next to its `(`, its
// `)` and each `|` between alternatives; when more text is glued after the
// group, the group is one glob group and keeps every blank (#578). Each
// source sits in a function body, where `zsh -f -n` checks only the lexer
// (#287), and the patterns are compared through the printer.
func TestZshCaseGroupBlank(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		pat  string
		want string // patterns joined by "|"
	}{
		{"(a b)", "a b"},
		{"(a\tb)", "a\tb"},
		{"( a )", "a"},
		{"(a | b)", "a|b"},
		{"(a  )", "a"},
		{"( )", ""},
		{"(a| |b)", "a||b"},
		{"(a $x)", "a $x"},
		{"($x )", "$x"},
		{"(a$x )", "a$x"},
		{"(a 'b' )", "a 'b'"},
		{"(a (b))", "a (b)"},
		{"(a ( b))", "a ( b)"},
		{"(a #b)", "a #b"},
		{"( #i)", "#i"},
		{"(*# SKIP*)", "*# SKIP*"},
		{`(\# Not a target*|\# environment*)`, `\# Not a target*|\# environment*`},
		{"((#b)([0-9a-f]##) ##(*))", "(#b)([0-9a-f]##) ##(*)"},
		{"(''(x|exec) *)", "''(x|exec) *"},
		{"(a[x] b)", "a[x] b"},
		{"(a[$x] b)", "a[$x] b"},
		{"(a b)|c)", "(a b)|c"},
		{"(a b))", "(a b)"},
		{"(a | b))", "(a | b)"},
		{"( #i)x)", "( #i)x"},
		{"(a b)c)", "(a b)c"},
		{"(a\nb)", "a\nb"},
		{"(a|\n b)", "a|\n b"},
		{"(a \n|b)", "a \n|b"},
		{"(a[\nb]*)", "a[\nb]*"},
		{"(a # c\n)", "a # c\n"},
	} {
		src := "f() {\ncase $x in\n  " + tt.pat + " print a ;;\nesac\n}\n"
		for mode, wrap := range flagArgReaders {
			f, err := NewParser(Variant(LangZsh)).Parse(wrap(strings.NewReader(src)), "")
			if err != nil {
				t.Errorf("%q %s: %v", tt.pat, mode, err)
				continue
			}
			if got := casePatterns(t, f); got != tt.want {
				t.Errorf("%q %s: patterns %q, want %q", tt.pat, mode, got, tt.want)
			}
		}
	}
}

// Zsh rejects each of these: a word that keeps going after the item's group
// closes, a `;`, `&` or `<` inside the group, a group that never closes, and
// a blank in a pattern without the opening `(` (#578).
func TestZshCaseGroupBlankErrors(t *testing.T) {
	t.Parallel()
	for _, pat := range []string{
		"(a b)c",
		"(a) b)",
		"(a ;b)",
		"(a &b)",
		"(a <b)",
		"(a b",
		"a b)",
		"(x)y[ z])",
		"(a[$x)y b)",
	} {
		src := "f() {\ncase $x in\n  " + pat + " print a ;;\nesac\n}\n"
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err == nil {
			t.Errorf("%q: parsed, want an error", pat)
		}
	}
}

// Stripping a blank moves the literal's position with its text, also across
// a line continuation among the blanks, and leaves no empty part beside an
// expansion (#578).
func TestZshCaseGroupBlankPositions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src        string
		parts      int
		start, end string // line:column of the pattern's ends
	}{
		{"case x in ( a ) :;; esac", 1, "1:13", "1:14"},
		{"case x in (a  ) :;; esac", 1, "1:12", "1:13"},
		{"case x in ($x ) :;; esac", 1, "1:12", "1:14"},
		{"case x in ( $x) :;; esac", 1, "1:13", "1:15"},
		{"case x in (a $x ) :;; esac", 2, "1:12", "1:16"},
		{"case x in (a \\\n) :;; esac", 1, "1:12", "1:13"},
		{"case x in ( \\\n  a) :;; esac", 1, "2:3", "2:4"},
		{"case x in (a\\\n ) :;; esac", 1, "1:12", "1:13"},
		{"case x in (a \\\n b) :;; esac", 2, "1:12", "2:3"},
	} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tt.src), "")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		ci := f.Stmts[0].Cmd.(*CaseClause).Items[0]
		w := ci.Patterns[0]
		if len(ci.Patterns) != 1 || len(w.Parts) != tt.parts {
			t.Errorf("%q: %d patterns, %d parts, want 1 and %d", tt.src, len(ci.Patterns), len(w.Parts), tt.parts)
			continue
		}
		if got := w.Pos().String(); got != tt.start {
			t.Errorf("%q: pattern starts at %s, want %s", tt.src, got, tt.start)
		}
		if got := w.End().String(); got != tt.end {
			t.Errorf("%q: pattern ends at %s, want %s", tt.src, got, tt.end)
		}
	}
}

// Every printer mode writes a case item read under #578 so that it parses
// back to the same patterns: a lone empty pattern prints as `( )`, an
// alternative that begins with `#` keeps the opening parenthesis so the `#`
// is not a comment, and text never follows the `)` of an opened pattern
// directly, where Zsh would read it as more of the pattern.
func TestZshCaseGroupBlankPrint(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"case x in (a b) print c ;; esac",
		"case x in ( ) print c ;; esac",
		"case x in (\t) print c ;; esac",
		"case x in ( #i) print c ;; esac",
		"case x in ( #) print c ;; esac",
		"case x in (\t#aa) print c ;; esac",
		"case x in (a|  #) print c ;; esac",
		"case x in (a | b)) print c ;; esac",
		"case x in ( #i)x) print c ;; esac",
		"case x in a) print c ;;\n(c d) ;; esac",
		"case x in a) print c ;;\n( ) ;; esac",
	} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\n"), "")
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		want := casePatterns(t, f)
		for _, mode := range []struct {
			name string
			opts []PrinterOption
		}{
			{"plain", nil},
			{"minify", []PrinterOption{Minify(true)}},
			{"single-line", []PrinterOption{SingleLine(true)}},
		} {
			var sb strings.Builder
			if err := NewPrinter(mode.opts...).Print(&sb, f); err != nil {
				t.Fatal(err)
			}
			f2, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(sb.String()), "")
			if err != nil {
				t.Errorf("%q %s: reprint %q: %v", src, mode.name, sb.String(), err)
				continue
			}
			if got := casePatterns(t, f2); got != want {
				t.Errorf("%q %s: reprint %q has patterns %q, want %q", src, mode.name, sb.String(), got, want)
			}
		}
	}
}

// The whole-group reading ends with the item's pattern list: a blank in the
// body, in a command substitution inside the pattern, or after a group glued
// to more text still separates words (#578).
func TestZshCaseGroupBlankScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		src  string
		want []string // each command's words, each word in <...>
	}{
		{"case x in (a b) print c d ;; esac", []string{"<print><c><d>"}},
		{"case x in (a b)\n  print c d ;; esac", []string{"<print><c><d>"}},
		{"case x in (a $(print b c)) :;; esac", []string{"<print><b><c>", "<:>"}},
		{"case x in (a b) print c ;; (d e) print f g ;; esac", []string{"<print><c>", "<print><f><g>"}},
		{"case x in a) print c d ;; esac", []string{"<print><c><d>"}},
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
}

// A blank the lexer keeps as text is always consumed by the literal reader,
// so no shape stalls it: a carriage return, a group cut off by EOF, and a
// blank before a `)` that ends the input (#578).
func TestZshCaseGroupBlankTerminates(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"case x in (a \r b) :;; esac",
		"case x in (a[$x\r b]) :;; esac",
		"case x in ( ",
		"case x in (a \n",
		"case x in (a )",
		"case x in ($x \t\n )",
	} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: parse did not finish", src)
		}
	}
}

// casePatterns prints the patterns of every case item in f, joined by "|".
func casePatterns(t *testing.T, f *File) string {
	t.Helper()
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
	return strings.Join(pats, "|")
}
