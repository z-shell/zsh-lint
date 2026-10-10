package syntax

import (
	"reflect"
	"strings"
	"testing"
)

// zshEmptySublistRow expands a row of the empty sublist tables. In a row, `@`
// stands for a `;` that ends an empty sublist, and `%&` and `%|` for a `&&`
// or `||` that ends its list. src is the source Zsh reads; blanked is the
// same source with each `@` replaced by a blank and each marked operator by
// `; `, which keeps every offset and holds no empty sublist.
func zshEmptySublistRow(row string) (src, blanked string) {
	src = strings.NewReplacer("@", ";", "%&", "&&", "%|", "||").Replace(row)
	blanked = strings.NewReplacer("@", " ", "%&", "; ", "%|", "; ").Replace(row)
	return src, blanked
}

// zshEmptySublistAccepts are native-valid rows (`zsh -f -n`, judged as files
// by empty standard error). They come from the internal/parse adapters this
// fork change replaced: the leading `;` after `do` (zsh-lint #238) and after
// `then` or `else` (#297), the redundant `;` (#332, #467, #569), and the
// dangling `&&` or `||` (#327, #466, #469, #548).
var zshEmptySublistAccepts = []string{
	// After `do`.
	"while (( $# )); do@ shift; done",
	"for x in a b; do@ print $x; done",
	"until (( $# )); do@ shift; done",
	"select o in a b c; do@ print $o; break; done",
	"for ((i = 0; i < 2; i++)); do@ print $i; done",
	"repeat 2 do@ print x; done",
	"repeat 2; do@ print x; done",
	"repeat $(cat n) do@ print x; done",
	"freload() { while (( $# )); do@ unfunction $1; autoload -U $1; shift; done }",
	"while true; do@break; done",
	"while true; do @ break; done",
	"while true; do\t@break; done",
	"while true; do@ @ break; done",
	"while true; do\n@ break; done",
	"while true; do@\n@ break; done",
	"while true; do # comment\n@ break; done",
	"while true; do@ # comment\nbreak; done",
	"while true; do@ while true; do@ break; done; break; done",
	"while true; do@ break; done; while true; do@ break; done",
	"while true; do@ break; done | cat",
	"while true; do@ print $(while true; do@ break; done); break; done",
	"for x in do; do@ print $x; done",
	"while print do; do@ break; done",
	"while true; do@ f() { @ }; done",
	"while true; do@ cat <<EOF\ndo;\nEOF\nbreak; done",
	"while true; do@ print 'do; x' \"do; y\"; break; done",
	"while true; do@ done\nwhile true; do@ break; done",
	"foreach i (a) do@ print $i; done",

	// After `then` and `else`.
	"if true; then@ print x; fi",
	"if true; then x; else@ y; fi",
	"if true; then x; elif false; then@ y; fi",
	"if true; then@ x; else@ y; fi",
	"if true; then@ x; elif false; then@ y; else@ z; fi",
	"if true; then@print x; fi",
	"if true; then @ print x; fi",
	"if true; then\t@print x; fi",
	"if true; then@ @ print x; fi",
	"if true; then\n@ print x; fi",
	"if true; then@\n@ print x; fi",
	"if true; then # comment\n@ print x; fi",
	"if true; then@ # comment\nprint x; fi",
	"if true; then x; else\n@ print y; fi",
	"if true; then@ if true; then@ print x; fi; fi",
	"while true; do@ if true; then@ print x; fi; break; done",
	"if true; then@ while true; do@ break; done; fi",
	"if true; then@ print x; fi && print z",
	"for x in then; do print $x; done\nif print then; then@ print x; fi",
	"if true; then@ print 'then; x' \"then; y\"; fi",
	"if true; then@ cat <<EOF\nthen;\nEOF\nprint x; fi",
	"if true; then@ fi\nif true; then@ else@ fi",
	"if true; then@ print $(if true; then@ print x; fi); fi",

	// Where a statement may start, and at the start of every list.
	"@",
	"@ @ @",
	"@ print a",
	"print x; @",
	"print run; @ print again",
	"print a & @ print b",
	"print a &! @ print b",
	"print a &| @ print b",
	"print a ;\n@\nprint b",
	"while true; @ do print; done",
	"while @ true; do print; done",
	"if @ true; then print; fi",
	"f() { while true; @ }",
	"f() { print body; @ }",
	"f() { : ; @ }",
	"f() { print a; @ print b; @ }",
	"f() { @ print a }",
	"print 'quoted;' ; @",
	`print "also;" ; @`,
	"while true;\n@",
	"print a; \\\n@",
	"print a & \\\n@",
	"print a; \\\n\\\n@",
	"\\\n@\nprint a",
	"( @ print a )",
	"{ @ print a }",
	"{ @ }",
	"case x in a) @ print ;; esac",
	"case x in a) @ ;; esac",
	"print <( @ print a )",
	`echo "$( @ )"`,
	`echo "$( @ print a )"`,
	`x="$( @ print a )"; print $x`,
	`f() { echo "$( @ print a )"; }; f`,
	`echo "a $( @ print b ) c"`,
	`echo "$(print a; @ print b)"`,
	`echo "$(echo "$( @ print a )")"`,
	"echo `@`",
	"echo `@ print a`",
	"echo \"`@ print a`\"",
	`echo "${x:-y} $( @ print a )"`,
	"echo $( @ print a )",
	"echo `print a; @ print $'b'`",
	"echo `print a; @ cat <<<b`",
	"echo `print a; @ cat <<E\nx\nE\n`",
	"echo `print $((1<<2)); @ print a`",
	"echo `print a; @ print ${x#$'b'}`",
	"echo `print a; @ x=$'b'`",
	"print x; @ echo `print $'b'; @ print c`",
	"@ echo `print a; @ print $'b'`",
	"echo `repeat 2 do print a; @ done`",
	"echo `repeat 2 { : ; @ }`",
	"f() { echo `repeat 2 { : ; @ }`; }",
	`print a\>& @ print b`,
	`f() { print <->& @ print b; }`,
	`print <1-3>& @ print b`,
	`print <->&| @ print b`,
	"echo `print a\\>& @ print b`",
	`f() { print a\>&| @ print b; }`,
	`print a\<& @ print b`,
	`: \<& @ :`,
	`if true; then print a\>& @ fi`,
	`print a\>& @ @`,
	`print $x\<& @ print b`,
	"print a\\>&\t@ print b",
	// A `;` inside a word is text, never an empty sublist (#572).
	"@ print ${x:-\n;}",
	"@ print $'a\\'; ;'",
	"echo \"$( @ print ${x:-\n;} )\"",
	"echo `@ print a # (;`\nprint b",
	"@ echo \"$( print a # (;\n)\"",
	"@ echo \"`print \\\"a; ;b\\\"`\"",
	`echo "$( @ print 'x;' )"`,
	"@ cat <<'A'\n\"$( ; lit )\"\nA",

	// A dangling `&&` or `||`.
	"x=0\nwhile (( x )) %&\n",
	"x=0\nwhile (( x )) %|\n",
	"x=0\nuntil (( x )) %&\n",
	"print a %&\n",
	"print a %|\n",
	"print a %& #bar\n",
	"print a %&\n@\n",
	"print a %& @\n",
	"print a %& @ @\n",
	"h() { print a %&\n}\n",
	"( print a %&\n)\n",
	"( print a %| )\n",
	"if true; then\nprint a %&\nfi\n",
	"while (( 0 )); do\nprint a %&\ndone\n",
	"case x in y) print a %&\n;; esac\n",
	"case x in y) print a %& ;& esac\n",
	"print a &&\nprint b %&\n",
	"h() {\nif true; then\nprint a %&\nfi\n}\n",
	"h() { print a %& }\n",
	"while true; do print a %& done\n",
	"case x in y) print a %& ;; esac\n",
	"if true; then\nprint a %&\nelif false; then\nprint b\nfi\n",
	"if true; then\nprint a %&\nelse\nprint b\nfi\n",
	"if print a %& then print b; fi\n",
	"while print a %& do print b; done\n",
	"foreach i (a) print a %& end\n",
	"print a && print b %|\n",
	"print $( print a %& )\n",
	"if true; then\nprint a %| \\\nelse\nprint b\nfi\n",
	"if true; then print a %& \\\nelif true; then :; fi\n",
	"if true; then print a %| \\\nfi\n",
	"while false; do print a %| \\\ndone\n",
	"{ print a %| \\\n}\n",
	"if print a %| \\\nthen print b; fi\n",
	"( print a %| \\\n)\n",
	"case x in x) print a %| \\\n;; esac\n",
	"print a %| \\\n",
	"print a %| \\\n@",
	"{ print a %| \\\n@ }",
	"if true; then print a %|\\\nelse print b; fi\n",
	"if true; then print a %| \\\n\nelse print b; fi\n",
	"{ print a %& \\\n\\\n\\\n}\n",
	"x=$(print a %| \\\n)\n",
	"if true; then print a %| # c\nelse print b; fi\n",
	"if true; then print a %|#c\nelse print b; fi\n",
	"if true; then print a %|\n# c ||\nelse print b; fi\n",
	"{ print a %| # c\n}\n",
	"{ print a %| \\\n# c\n}\n",
	"if true; then print a %| # c \\\nelse print b; fi\n",
	"if true; then print ${#x} %| \\\nelse print b; fi\n",
	"if true; then print \"a\nb\" %| \\\nelse print b; fi\n",
	"if true; then print 'a\n# x' %|\nelse print b; fi\n",
	"x=`print a %&`\n",
	"x=`print a %|`\n",
	"x=`print a %| `\n",
	"x=`print a %|\n`\n",
	"x=`print a %| # c\n`\n",
	"x=`print a %| \\\n`\n",
	"x=\"`print a %|`\"\n",
	"print `print a %|` b\n",
	"x=`print a; print b %|`\n",
	"x=$(print `print a %|`)\n",
	"x=`print \\`print a %|\\``\n",
	"x=`print \\`print a\\` %|`\n",
	"x=`print a %|` || print c\n",
	"x=`print a %|` %|\n",
	"x=`print a %|`; y=`print b %&`\n",
	"f() { x=`print a %|`; }\n",
	"print ${x:-`print a %|`}\n",
	"print `print a %& @ `\n",
	"{ print a %& @ }\n",
	"( print a %& @ )\n",
	"if print a %& @ then :; fi\n",
	"if true; then print a %& @ elif true; then :; fi\n",
	"if true; then print a %& @ else :; fi\n",
	"if true; then print a %& @ fi\n",
	"while print a %& @ do break; done\n",
	"for x in a; do print a %| @ done\n",
	"case x in x) print a %& @ esac\n",
	"case x in x) print a %& @ ;; esac\n",
	"foreach x (a)\nprint a %& @\nend\n",
}

// TestZshEmptySublist checks each native-valid row parses, and to the tree of
// its blanked form: an empty sublist owns no node, and a dangling operator
// leaves its left operand as a statement ended where the operator stands.
func TestZshEmptySublist(t *testing.T) {
	t.Parallel()

	p := NewParser(Variant(LangZsh), KeepComments(true))
	for _, row := range zshEmptySublistAccepts {
		src, blanked := zshEmptySublistRow(row)
		t.Run(src, func(t *testing.T) {
			got, err := p.Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", src, err)
			}
			want, err := p.Parse(strings.NewReader(blanked), "")
			if err != nil {
				t.Fatalf("Parse(%q) of the blanked form failed: %v", blanked, err)
			}
			if !reflect.DeepEqual(got, want) {
				var g, w strings.Builder
				if err := NewPrinter().Print(&g, got); err != nil {
					t.Fatal(err)
				}
				if err := NewPrinter().Print(&w, want); err != nil {
					t.Fatal(err)
				}
				t.Errorf("tree of %q differs from the tree of %q:\n got %q\nwant %q", src, blanked, g.String(), w.String())
			}
		})
	}
}

// TestZshEmptySublistRejects checks that native-invalid rows (`zsh -f -n`
// reports a parse error, except where noted) stay errors. A `;` is an empty sublist only where a
// statement may start: not after a redirection operator, a pipe, `!` or
// `[[`, nor inside a word, a glob group or an arithmetic expression. A `&`,
// another operator or a case terminator outside a case does not end the list
// of a `&&` or `||`.
func TestZshEmptySublistRejects(t *testing.T) {
	t.Parallel()

	tests := []struct{ src, want string }{
		{"; &\n", "1:3: `&` can only immediately follow a statement"},
		{"print a; ; &\n", "1:12: `&` can only immediately follow a statement"},
		{";;\n", "1:1: `;;` can only be used in a case clause"},
		{"; ;;\n", "1:3: `;;` can only be used in a case clause"},
		{"true; &\n", "1:7: `&` can only immediately follow a statement"},
		{"true | ;\n", "1:6: `|` must be followed by a statement"},
		{"print a | \\\n;\n", "1:9: `|` must be followed by a statement"},
		{"if true; ;\n", "1:1: `if <cond>` must be followed by `then`"},
		{"while true; do; & break; done\n", "1:17: `&` can only immediately follow a statement"},
		{"do; print x\n", "1:1: `do` can only be used in a loop"},
		{"while true; do;; break; done\n", "1:15: `;;` can only be used in a case clause"},
		{"while true; do; print x\n", "1:1: `while` statement must end with `done`"},
		{"if true; then; & print x; fi\n", "1:16: `&` can only immediately follow a statement"},
		{"if true; then;; print x; fi\n", "1:14: `;;` can only be used in a case clause"},
		{"if true; then x; else;; y; fi\n", "1:22: `;;` can only be used in a case clause"},
		{"then; print x\n", "1:1: `then` can only be used in an `if`"},
		{"if true; then; print x\n", "1:1: `if` statement must end with `fi`"},
		{"; print a(;)\n", "1:12: `)` can only be used to close a subshell"},
		{"; print *(;)\n", "1:12: `)` can only be used to close a subshell"},
		{"; print a(|;)\n", "1:13: `)` can only be used to close a subshell"},
		{"; [[ a == (;) ]]\n", "1:12: not a valid test operator: `;`"},
		{"[[ ; a == b ]]\n", "1:1: `[[` must be followed by an expression"},
		{`echo "$( ; [[ a || ; b ]] )"` + "\n", "1:17: `||` must be followed by an expression"},
		{`echo "$( ; [[ ( ; a ) ]] )"` + "\n", "1:15: `(` must be followed by an expression"},
		{"echo `; case a in x | ; y) ;; esac`\n", "1:23: case patterns must consist of words"},
		{"; print a > ( ; print b)\n", "1:24: a command can only contain words and redirects; encountered `)`"},
		{"print a >& ; print b\n", "1:9: `>&` must be followed by a word"},
		{`echo "$(true | ; )"` + "\n", "1:14: `|` must be followed by a statement"},
		{"echo `true | ;`\n", "1:12: `|` must be followed by a statement"},
		{`echo "$(if true; ; )"` + "\n", "1:9: `if <cond>` must be followed by `then`"},
		{"echo `; print a(;)`\n", "1:18: `)` can only be used to close a subshell"},
		{"print a && &\n", "1:9: `&&` must be followed by a statement"},
		{"print a && & print b\n", "1:9: `&&` must be followed by a statement"},
		{"print a && && print b\n", "1:9: `&&` must be followed by a statement"},
		{"print a &&\n||\n", "1:9: `&&` must be followed by a statement"},
		{"print a && | b\n", "1:9: `&&` must be followed by a statement"},
		{"print a || ;;\n", "1:9: `||` must be followed by a statement"},
		{"print a && ; &\n", "1:9: `&&` must be followed by a statement"},
		{"print a && ; && print b\n", "1:9: `&&` must be followed by a statement"},
		{"x=0\nwhile (( x )) |\n", "2:15: `|` must be followed by a statement"},
		{"x=0\nif (( x )) &&\n", "2:1: `if <cond>` must be followed by `then`"},
		{"&& print a\n", "1:1: `&&` can only immediately follow a statement"},
		{"print a &&\n}\n", "2:1: `}` can only be used to close a block"},
		{"print a &&\nfi\n", "2:1: `fi` can only be used to end an `if`"},
		{"print a &&\ndone\n", "2:1: `done` can only be used to end a loop"},
		{"( print a && ; ) )\n", "1:18: statements must be separated by &, ; or a newline"},
		{"print a || \\\n| print b\n", "1:9: `||` must be followed by a statement"},
		{"print a && \\\n&\n", "1:9: `&&` must be followed by a statement"},
		{"{ print a && \\\n&& }\n", "1:11: `&&` must be followed by a statement"},
		{"if true; then print a | \\\nelse print b; fi\n", "2:1: `else` can only be used in an `if`"},
		{"{ print a | # c\n}\n", "2:1: `}` can only be used to close a block"},
		// `zsh -f -n` does not read the backquoted body of an assignment;
		// these three fail when `zsh -f` runs them.
		{"x=`print a | `\n", "1:12: `|` must be followed by a statement"},
		{"x=`print a || && b`\n", "1:12: `||` must be followed by a statement"},
		{"x=`print a || &`\n", "1:12: `||` must be followed by a statement"},
		{"x=`print a ||\n", "1:3: reached EOF without closing quote \"`\""},
		{"print a || `\n", "1:12: reached EOF without closing quote \"`\""},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			_, err := p.Parse(strings.NewReader(tc.src), "")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want %q", tc.src, tc.want)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("Parse(%q) error %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestEmptySublistDialectGate checks that only Zsh reads an empty sublist or
// a dangling operator; the other dialects keep upstream's errors.
func TestEmptySublistDialectGate(t *testing.T) {
	t.Parallel()

	for _, lang := range []LangVariant{LangBash, LangPOSIX, LangMirBSDKorn} {
		t.Run(lang.String(), func(t *testing.T) {
			for _, src := range []string{
				"; print a\n",
				"print a; ; print b\n",
				"while true; do; break; done\n",
				"if true; then; print a; fi\n",
				"print a &&\n",
				"{ print a || ; }\n",
				"( print a && )\n",
			} {
				if _, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), ""); err == nil {
					t.Errorf("Parse(%q) succeeded, want an error", src)
				}
			}
		})
	}
}
