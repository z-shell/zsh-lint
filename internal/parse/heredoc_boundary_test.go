package parse

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// #703: only the true substitution close drops its pending documents.
// These are native Zsh test fixtures at the project's 5.9.2 baseline.
// https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
func TestHeredocSubstitutionBoundaryAdapters(t *testing.T) {
	for _, prefix := range []string{
		"print $(cat <<EOF)\n",
		"print <(cat <<EOF)\n",
		"print >(cat <<EOF)\n",
		"print =(cat <<EOF)\n",
		"print `cat <<EOF`\n",
		"print `print \\`cat <<EOF\\``\n",
		"print `cat <<EOF; true`\n",
		"print $(cat <<EOF; true)\n",
		"print $( (cat <<EOF) )\n",
		"print $( (cat <<EOF)\nbody ) }\nEOF\n)\n",
		"print $(case x in (x) cat <<EOF;; esac)\n",
		"print $(cat <<EOF) ')'\n",
		"print $(echo 'a)'; cat <<EOF)\n",
		"print $(cat <<EOF \\); true)\n",
		"print $(cat <<A <<B)\n",
		"cat <<EOF; print $(cat <<X)\nouter ) }\nEOF\n",
		"print $(cat <<EOF\nbody ) }\nEOF\n)\n",
		"cat <<EOF\nbody ) }\nEOF\n",
	} {
		for _, site := range []string{"{ true } always { true }", "{ local x=1 }", "[[ x == ((a|b)|c) ]]"} {
			t.Run(prefix+site, func(t *testing.T) {
				src := "echo before\n" + prefix + "# retained\n" + site + "\necho after\n"
				path := filepath.Join(t.TempDir(), "row.zsh")
				if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.Command("zsh", "-f", "-n", path).CombinedOutput(); err != nil || len(output) > 0 {
					t.Fatalf("native: %v: %s", err, output)
				}
				file, err := Parse(strings.NewReader(src), "boundary.zsh")
				if err != nil {
					t.Fatal(err)
				}
				stmts := file.AST().Stmts
				if got := nodeText(src, stmts[0]); got != "echo before" {
					t.Fatalf("first = %q", got)
				}
				if got := nodeText(src, stmts[len(stmts)-1]); got != "echo after" {
					t.Fatalf("last = %q", got)
				}
				var docs, comments, sites int
				syntax.Walk(file.AST(), func(n syntax.Node) bool {
					switch n := n.(type) {
					case *syntax.Comment:
						if n.Text == " retained" {
							comments++
						}
					case *syntax.Redirect:
						if n.Op == syntax.Hdoc {
							docs++
							if n.Hdoc != nil {
								body := heredocBody(t, n)
								if body != "body ) }\n" && body != "outer ) }\n" {
									t.Errorf("body = %q", body)
								}
								if !n.ClosePos.IsValid() {
									t.Error("lost real closer")
								}
							} else if n.ClosePos.IsValid() {
								t.Error("bodyless document gained closer")
							}
						}
					case *syntax.DeclClause:
						if nodeText(src, n) != "local x=1" {
							t.Errorf("declaration = %q", nodeText(src, n))
						}
						sites++
					case *syntax.BinaryTest:
						if nodeText(src, n.Y) != "((a|b)|c)" {
							t.Errorf("pattern = %q", nodeText(src, n.Y))
						}
						sites++
					case *syntax.Block:
						if len(n.Stmts) == 1 && nodeText(src, n.Stmts[0]) == "true" {
							sites++
						}
					}
					return true
				})
				if docs != strings.Count(prefix, "<<") || comments != 1 || sites == 0 {
					t.Fatalf("docs/comments/sites = %d/%d/%d", docs, comments, sites)
				}
				_, err = Parse(strings.NewReader(src+")\n"), "error.zsh")
				var pe syntax.ParseError
				if !errors.As(err, &pe) || int(pe.Pos.Offset()) != len(src) {
					t.Fatalf("later error = %v, want byte %d", err, len(src))
				}
			})
		}
	}
}

func TestHeredocUnstartedAtEOFAdapters(t *testing.T) {
	for _, src := range []string{
		"{ true } always { true }; cat <<EOF",
		"{ local x=1 }; cat <<EOF",
		"[[ x == ((a|b)|c) ]]; cat <<EOF",
	} {
		if _, err := Parse(strings.NewReader(src), "eof.zsh"); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// A closing backquote is a reserved-word boundary; an opening substitution
// continuing a pattern word is not. Both paths must retain body ownership.
func TestHeredocCaseBacktickBoundaryAdapters(t *testing.T) {
	for _, prefix := range []string{
		"x=`case a in a) :;; esac`\n",
		"x=\"`case a in a) :;; esac`\"\n",
		"x=`case a in a) :;; esac;`\n",
		"x=`case a in a) :;; esac `\n",
		"x=$(case a in a) :;; esac)\n",
		"x=`echo \\`case a in a) :;; esac\\``\n",
		"case a in esac`echo x`) :;; a) :;; esac\n",

		"case a in esac\\`) :;; a) :;; esac\n",
		"case a in 'esac`') :;; a) :;; esac\n",
	} {
		for _, doc := range []string{": <<<x\n", "cat <<EOF\nbody\nEOF\n", "cat <<-$'\\tEOF'\nbody\n\tEOF\n"} {
			for _, site := range []string{"{ true } always { true }", "f() { local y=1 }"} {
				src := prefix + doc + site + "\n"
				t.Run(src, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "case.zsh")
					if err := os.WriteFile(path, []byte(src), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command("zsh", "-f", "-n", path).CombinedOutput(); err != nil || len(out) > 0 {
						t.Fatalf("native %v: %s", err, out)
					}
					if _, err := Parse(strings.NewReader(src), "case.zsh"); err != nil {
						t.Fatal(err)
					}
					if _, ok := sourceHeredocSkips([]byte(src)); !ok {
						t.Fatal("scanner failed closed")
					}
				})
			}
		}
	}
}

// Known-invalid source is never executed. A failed lexical scan must retain
// the primary diagnostic, even when a speculative adapter retry would expose
// the earlier unmatched substitution instead (#703).
func TestHeredocUnclosedSubstitutionFailsClosed(t *testing.T) {
	const src = "echo $(cat <<EOF\nbody\nEOF\n{ true } always { true }\n"
	_, primary := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src), "unclosed.zsh")
	_, got := Parse(strings.NewReader(src), "unclosed.zsh")
	if primary == nil || got == nil || primary.Error() != got.Error() {
		t.Fatalf("primary=%v; adapter=%v", primary, got)
	}
	if _, ok := sourceHeredocSkips([]byte(src)); ok {
		t.Fatal("accepted an unclosed lexical frame")
	}
	t.Log(got)
}

// The fork separately rejects this native-valid nested pattern word before
// adapters run. Exercise the scanner directly so that its closing-delimiter
// rule cannot mistake the opening escaped backquote for the end of esac.
func TestHeredocCasePatternOpeningBacktick(t *testing.T) {
	const src = "x=`case a in esac\\`echo x\\`) :;; a) :;; esac`\ncat <<EOF\nbody\nEOF\n{ true } always { true }\n"
	skips, ok := sourceHeredocSkips([]byte(src))
	if !ok {
		t.Fatal("scanner rejected an opening backquote in a pattern word")
	}
	bodyNewline := strings.Index(src, "\nbody\n")
	wantEnd := strings.Index(src, "{ true }")
	if skips[bodyNewline] != wantEnd {
		t.Fatalf("body skip=%d, want %d", skips[bodyNewline], wantEnd)
	}
}
