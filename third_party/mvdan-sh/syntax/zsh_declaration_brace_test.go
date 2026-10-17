package syntax

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"
)

// Zsh reserves a whole-word `}` even in a declaration's argument list.
// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
func TestZshDeclarationBraceTermination(t *testing.T) {
	p := NewParser(Variant(LangZsh), KeepComments(true))
	for _, name := range []string{"local", "typeset", "declare", "export", "readonly", "integer", "float", "let"} {
		for _, wrap := range []string{
			"if [[ -n x ]] { %s u=1 }",
			"for i in 1; { %s u=1 }",
			"while (( 0 )) { %s u=1 }",
			"{ %s u=1 }",
			"f() { %s u=1 }",
			`: "$({ %s u=1 })"`,
		} {
			src := strings.Replace(wrap, "%s", name, 1) + "\n"
			t.Run(src, func(t *testing.T) {
				// Reuse the parser and force token reads across input chunks.
				f, err := p.Parse(iotest.OneByteReader(strings.NewReader(src)), "")
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				Walk(f, func(n Node) bool {
					var cmd Command
					switch n := n.(type) {
					case *DeclClause:
						if n.Variant.Value != name || len(n.Args) != 1 || n.Args[0].Name.Value != "u" {
							t.Fatalf("declaration changed: %#v", n)
						}
						cmd = n
					case *LetClause:
						if name != "let" || len(n.Exprs) != 1 {
							t.Fatalf("let changed: %#v", n)
						}
						cmd = n
					}
					if cmd != nil {
						count++
						start := strings.Index(src, name+" u=1")
						if cmd.Pos().Offset() != uint(start) || cmd.End().Offset() != uint(start+len(name+" u=1")) {
							t.Fatalf("clause span = %s-%s", cmd.Pos(), cmd.End())
						}
					}
					return true
				})
				if count != 1 {
					t.Fatalf("walked %d declarations, want one", count)
				}
				var out bytes.Buffer
				if err := NewPrinter().Print(&out, f); err != nil {
					t.Fatal(err)
				}
				if _, err := p.Parse(newStrictReader(out.String()), ""); err != nil {
					t.Fatalf("round trip %q: %v", out.String(), err)
				}
			})
		}
	}
}

func TestZshDeclarationBraceWords(t *testing.T) {
	p := NewParser(Variant(LangZsh))
	for _, name := range []string{"local", "typeset", "declare", "export", "readonly", "integer", "float"} {
		for _, tail := range []string{` '}'`, ` "}"`, ` \}`, ` x=}`, ` x={a}`, ` x=${y}`, ` x=(\})`} {
			src := "{ " + name + tail + " }"
			f, err := p.Parse(iotest.OneByteReader(strings.NewReader(src)), "")
			if err != nil {
				t.Fatalf("%q: %v", src, err)
			}
			b := f.Stmts[0].Cmd.(*Block)
			d := b.Stmts[0].Cmd.(*DeclClause)
			if len(d.Args) != 1 || b.Rbrace.Offset() != uint(len(src)-1) {
				t.Fatalf("%q: literal brace closed the block", src)
			}
		}
		for _, src := range []string{name + " }", "{ " + name + " x=1}"} {
			if _, err := p.Parse(strings.NewReader(src), ""); err == nil {
				t.Fatalf("invalid source %q accepted", src)
			}
		}
		// Reuse after an error, with a clause that has no arguments.
		if _, err := p.Parse(strings.NewReader("{ "+name+" }"), ""); err != nil {
			t.Fatal(err)
		}
	}
}
