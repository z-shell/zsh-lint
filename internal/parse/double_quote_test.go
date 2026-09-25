package parse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A double-quoted string holds substitutions with their own quoting, so its
// closing `"` is not the next `"` in the source (#393).
func TestSkipDoubleQuotedString(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		end  int // offset of the closing `"`, or -1 when not decidable
	}{
		{"plain", `"abc" x`, 4},
		{"escaped quote", `"a\"b" x`, 5},
		{"command substitution", `"$(print "it's")" x`, 16},
		{"single quote holding a double quote", `"$(print '"')" x`, 13},
		{"backquotes", "\"`print \"it's\"`\" x", 15},
		{"parameter default", `"${x:-"it's"}" x`, 13},
		{"single quote literal in quoted parameter", `"${x:-a'}" x`, 9},
		{"balanced braces in parameter", `"${x:-{a}}" x`, 10},
		{"escaped brace in parameter", `"${x:-a\}b}" x`, 11},
		{"arithmetic then quote", `"$(( 1 + 2 ))'" x`, 14},
		{"legacy arithmetic", `"$[1+2]'" x`, 8},
		{"paren in quoted body", `"$(print ")")" x`, 13},
		{"paren in single-quoted body", `"$(print ')')" x`, 13},
		{"subshell in substitution", `"$( (print "a'") )" x`, 18},
		{"nested substitution", `"$(print $(print "q'"))" x`, 23},
		{"ansi-c string in substitution", `"$(print $'it\'s')" x`, 18},
		{"comment in substitution", "\"$(print a # it's\n)\" x", 19},
		{"hash inside a word", `"$(print a#it)" x`, 14},
		{"escaped backquote", "\"`print \\`print q\\``\" x", 20},
		{"lone dollar", `"a$" x`, 3},
		{"ansi-c string in unquoted parameter in substitution", `"$(print ${x:-$'it\'s'})" x`, 24},
		{"quoted parameter closes at its first brace", `"${x:-{}" x`, 8},
		{"quoted parameter does not nest braces", `"${x:-{a}b}" x`, 11},
		{"unquoted parameter in substitution nests braces", `"$(print ${x:-{a}b})" x`, 20},
		{"glob flags in substitution", `"$( [[ a == (#b)(*) ]] && print "it's" )" x`, 40},
		{"glob qualifier in substitution", `"$(print -l x(#qN) "it's")" x`, 26},
		{"herestring in substitution", `"$(cat <<<"it's")" x`, 17},
		// Whether `#` opens a comment follows the grammar position of the
		// `(` before it (review of #399).
		{"comment right after the opener", "\"$(# c )\nprint \"it's\")\" x", 22},
		{"case as an argument", "\"$(print case \"it's\")\" x", 21},
		{"shift in an arithmetic command", "\"$( (( 1 << 2 )) && print \"it's\" )\" x", 34},
		{"comment in a subshell", "\"$( (# c )\nprint \"it's\") )\" x", 26},
		{"comment in a subshell after a separator", "\"$(print a; (# c )\nprint \"it's\") )\" x", 34},
		{"comment in a process substitution", "\"$(print <(# c )\nprint \"it's\") )\" x", 32},
		{"comment in an equals process substitution", "\"$(print =(# c )\nprint \"it's\") )\" x", 32},
		{"comment in a function body subshell", "\"$(f() (# c )\nprint \"it's\"); f)\" x", 31},
		{"comment in an array assignment", "\"$(a=(# c )\n y ); print \"it's\")\" x", 31},
		{"glob flags in an argument", "\"$(print a (#i)b \"it's\")\" x", 24},
		{"glob flags in an array element", "\"$(a=( (#i)x ); print \"it's\")\" x", 29},
		{"glob flags after a precommand modifier", "\"$(noglob print (#i)a \"it's\")\" x", 29},
		{"hash in a glob alternative", "\"$(print (a|#b) \"it's\")\" x", 23},
		{"glob flags in a condition alternative", "\"$([[ A == (x|(#i)a) ]] && print \"it's\")\" x", 40},
		{"comment in a subshell after time", "\"$(time (# c )\nprint \"it's\") )\" x", 30},
		{"glob flags after a redirection", "\"$(print a >&2 (#i)q \"it's\")\" x", 28},
		{"unbalanced paren in a process substitution comment", "\"$(print <(# c (\nprint \"it's\") )\" x", 32},
		{"case as a clobber redirection target", "\"$(print a >| case; print \"it's\")\" x", 33},
		{"case as an array element", "\"$(a=( case x ); print \"it's\")\" x", 30},
		// Not decidable here: the caller falls back to its own handling.
		{"unterminated", `"$(print "it's")`, -1},
		{"case in substitution", `"$(case a in a) print "q'";; esac)" x`, -1},
		{"heredoc in substitution", "\"$(cat <<E\nit's\nE\n)\" x", -1},
		{"heredoc body holding a closer", "\"$(cat <<E\nx ) '\nE\n)\" x", -1},
		{"case command after a separator", "\"$(print a; case a in (a) print \"q'\";; esac)\" x", -1},
		{"comment in a paren the scan cannot place", "\"$(\"print\" (#i)x\n) \"it's\")\" x", -1},
		{"quote in arithmetic", `"$(( '1' ))" x`, -1},
		// `##'` is the character code of a quote, not a quote: refused. Two
		// of them would pair up as one single-quoted string without the
		// refusal, and the scan would report an end it never checked.
		{"quote in an arithmetic command", `"$( (( x = ##' )) ; print "it's")" x`, -1},
		{"two quotes in arithmetic commands", `"$( (( x = ##' )); (( y = ##' )); print "it's")" x`, -1},
		{"unterminated backquote", "\"`print \" x", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end, ok := skipDoubleQuotedString([]byte(tc.src), 0)
			if tc.end < 0 {
				if ok {
					t.Fatalf("got end %d, want undecided", end)
				}
				return
			}
			if !ok || end != tc.end {
				t.Fatalf("got (%d, %v), want (%d, true)", end, ok, tc.end)
			}
		})
	}
}

// An odd quote inside a double-quoted substitution must not hide a later
// construct from any site scanner (#393). Every row passes `zsh -f -n`.
func TestQuoteInDoubleQuotedSubstitutionAccepted(t *testing.T) {
	lines := []string{
		`print "$(print "it's")"`,
		`print "$(print '"')"`,
		"print \"`print \"it's\"`\"",
		`print "${x:-"it's"}"`,
		`print "$(print "$(print "it's")")"`,
		`print "$(print ${x:-$'it\'s'})"`,
		`print "${x:-{}"`,
		`print "$( [[ a == (#b)(*) ]] && print "it's" )"`,
		`print "$(print -l x(#qN) "it's")"`,
		`print "$(cat <<<"it's")"`,
	}
	constructs := map[string]string{
		"alternate for":       "for x ( a b ) { print $x }",
		"alternate select":    "select x ( a ) { break }",
		"alternate if":        "if (( 1 )) { print a }",
		"alternate while":     "while (( 0 )) { print a }",
		"repeat":              "repeat 2 do print a; done",
		"try always":          "{ print a } always { print b }",
		"condition list":      "if f && [[ a ]] { print a }",
		"in function":         "f() {\n  for x ( a b ) { print $x }\n}",
		"declaration brace":   "f() { local a=b }",
		"empty sublist":       "if true; then ; print a; fi",
		"redundant separator": "; ; print a",
	}
	for _, s := range lines {
		for name, c := range constructs {
			src := s + "\n" + c + "\n"
			if _, err := parseWithAdapters([]byte(src), "t.zsh"); err != nil {
				t.Errorf("%s after %s: %v", name, s, err)
			}
		}
	}
}

// Invalid Zsh stays rejected: each testdata/invalid-393-*.txt source fails
// `zsh -f -n`, and the shared rule must not make an adapter accept it.
func TestQuoteInDoubleQuotedSubstitutionRejected(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/invalid-393-*.txt")
	if err != nil || len(fixtures) != 5 {
		t.Fatalf("invalid-393 fixtures = %v, %v; want 5", fixtures, err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-393.zsh")
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("Parse() error = %v, want a parse error for a source native Zsh rejects", err)
			}
		})
	}
}

// The rows #393 lists as parsing before the fix keep their verdict and their
// tree: the loop is still the ForClause at its own offset, over the same words.
func TestQuoteInDoubleQuotedSubstitutionKeepsVerdict(t *testing.T) {
	const loop = "for x ( a b ) { print $x }"
	for _, src := range []string{
		"print $(print \"it's\")\n" + loop + "\n",
		"print \"it's\"\n" + loop + "\n",
		"print \"$(print \"a\")\"\n" + loop + "\n",
		"print \"$(print \"it's\")\"\nprint \"$(print \"it's\")\"\n" + loop + "\n",
		loop + "\nprint \"$(print \"it's\")\"\n",
	} {
		file, err := parseWithAdapters([]byte(src), "t.zsh")
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		var loops []*syntax.ForClause
		syntax.Walk(file, func(node syntax.Node) bool {
			if f, ok := node.(*syntax.ForClause); ok {
				loops = append(loops, f)
			}
			return true
		})
		if len(loops) != 1 {
			t.Fatalf("%q: got %d for loops, want 1", src, len(loops))
		}
		if got, want := int(loops[0].Pos().Offset()), strings.Index(src, "for "); got != want {
			t.Errorf("%q: loop at offset %d, want %d", src, got, want)
		}
		iter, ok := loops[0].Loop.(*syntax.WordIter)
		if !ok || len(iter.Items) != 2 || iter.Items[0].Lit() != "a" || iter.Items[1].Lit() != "b" {
			t.Errorf("%q: loop words changed: %#v", src, loops[0].Loop)
		}
	}
}
