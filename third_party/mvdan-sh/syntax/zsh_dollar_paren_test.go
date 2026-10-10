package syntax

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestZshDollarParenReading(t *testing.T) {
	for _, tc := range []struct {
		src   string
		arith bool
	}{
		{`print $((( 1 )) && print ok)`, false},
		{`print "$((( 1 )) && print ok)"`, false},
		{`print $((print ok) | cat)`, false},
		{`print $((print ok) )`, false},
		{`print $((print \())`, false},
		{`print $((print \[))`, false},
		{`print $((print \]))`, false},
		{`echo $((foo) )`, false},
		{`echo $((echo a); (echo b))`, false},
		{`print $((print ')') | cat)`, false},
		{`print $((print "${x:-)}") | cat)`, false},
		{`print $((print ${x:-$(print 1)}) | cat)`, false},
		{`print $((print ${x:-$y}) | cat)`, false},
		{`print $((print $x) | cat)`, false},
		{`print $((print $((1))) | cat)`, false},
		{`print $((print ] [))`, false},
		{`print $((print "$(print ')')") | cat)`, false},
		{"print $((print ${x:-`print '$( ) )'`}) && print ok)", false},
		{`print $((1 + 2))`, true},
		{`print $(((1 + 2)))`, true},
		{`print "$((1 + 2))"`, true},
		{`print $((a[1] + ${x:-1}))`, true},
		{`print $(( ${x:-1)} + 2 ))`, true},
		{`print $(( ${x:-[1} + 2 ))`, true},
		{`print $(( ${x:-"1)"} + 2 ))`, true},
		{`print $(( $(print 1) + 2 ))`, true},
		{"print $(( `print '1'` + 2 ))", true},
		{"print \"$(( `print '1'` + 2 ))\"", true},
		{`print $(( $(case x in x) print 1;; esac) + 2 ))`, true},
		{"print $(( $(cat <<EOF\n)\nEOF\n) + 2 ))", true},
		{"print $(( ${x:-$(cat <<EOF\n})\nEOF\n)} + 2 ))", true},
		{`print $(( ${x:-$(case x in x) print '} )';; esac)} + 2 ))`, true},
		{"print $((" + strings.Repeat(" ", 4096) + "1 + 2))", true},
		{"print $((print " + strings.Repeat("x", 4096) + ") | cat)", false},
		{"print $(( $(print $((" + strings.Repeat(" ", 4096) + "1))) + 3 ))", true},
		{"print $((print $(print $((" + strings.Repeat(" ", 4096) + "1)))) | cat)", false},
	} {
		t.Run(tc.src, func(t *testing.T) {
			for _, slow := range []bool{false, true} {
				var r io.Reader = newStrictReader(tc.src + "\nprint after\n")
				if slow {
					r = iotest.OneByteReader(r)
				}
				f, err := NewParser(Variant(LangZsh)).Parse(r, "")
				if err != nil {
					t.Fatalf("slow=%v: %v", slow, err)
				}
				if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(len(tc.src)+1) {
					t.Fatalf("slow=%v: following statement lost or moved", slow)
				}
				part := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0]
				if dq, ok := part.(*DblQuoted); ok {
					part = dq.Parts[0]
				}
				if tc.arith {
					if _, ok := part.(*ArithmExp); !ok {
						t.Fatalf("slow=%v: want ArithmExp, got %T", slow, part)
					}
				} else {
					cs, ok := part.(*CmdSubst)
					if !ok {
						t.Fatalf("slow=%v: want CmdSubst, got %T", slow, part)
					}
					if cs.Left.Offset() != uint(strings.Index(tc.src, "$(")) || cs.Right.Offset() != uint(strings.LastIndex(tc.src, ")")) {
						t.Fatalf("slow=%v: substitution positions moved", slow)
					}
				}
			}
		})
	}
}

func TestZshDollarParenReadErrors(t *testing.T) {
	r := io.MultiReader(strings.NewReader("print $((1"+strings.Repeat(" ", 4096)), badReader{})
	if _, err := NewParser(Variant(LangZsh)).Parse(r, ""); !errors.Is(err, errBadReader) {
		t.Fatalf("lost input error: %v", err)
	}
	r = iotest.DataErrReader(strings.NewReader("print $((( 1 )) && print ok)"))
	f, err := NewParser(Variant(LangZsh)).Parse(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*CmdSubst); !ok {
		t.Fatal("lost command-substitution reading when EOF accompanies data")
	}
}

func TestZshDollarParenOneByteReplay(t *testing.T) {
	// The lookahead reads only the second closer from the next input buffer.
	r := io.MultiReader(strings.NewReader("print $((1)"), strings.NewReader(")"), strings.NewReader("\nprint after\n"))
	f, err := NewParser(Variant(LangZsh)).Parse(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != 13 {
		t.Fatal("one-byte replay lost or moved the following statement")
	}
	if _, ok := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ArithmExp); !ok {
		t.Fatal("one-byte replay lost the arithmetic reading")
	}
}

func TestZshDollarParenNestedCommands(t *testing.T) {
	const depth = 40
	src, quoted := "1", "1"
	for range depth {
		src = "$(( $(print " + src + ") + 1 ))"
		quoted = "$(( $(print \"" + quoted + "\") + 1 ))"
	}
	for _, tc := range []struct {
		src      string
		commands int
	}{
		{strings.Repeat("$((", depth) + "1" + strings.Repeat("))", depth), 0},
		{src, depth},
		{quoted, depth},
	} {
		for _, slow := range []bool{false, true} {
			var r io.Reader = newStrictReader("print " + tc.src + "\n")
			if slow {
				r = iotest.OneByteReader(r)
			}
			f, err := parseZshWithDeadline(t, r)
			if err != nil {
				t.Fatal(err)
			}
			arithmetic, commands := 0, 0
			Walk(f, func(node Node) bool {
				switch node := node.(type) {
				case *ArithmExp:
					arithmetic++
					if node.X == nil {
						t.Fatal("private lookahead discarded a real arithmetic expression")
					}
				case *CmdSubst:
					commands++
				}
				return true
			})
			if arithmetic != depth || commands != tc.commands {
				t.Fatalf("slow=%v: lost nested expansions: %d arithmetic, %d commands", slow, arithmetic, commands)
			}
		}
	}
}

func TestZshDollarParenIncompleteParameter(t *testing.T) {
	for _, src := range []string{"print $(( ${x:-", "print $(( ${x:-$(print x)"} {
		if _, err := parseZshWithDeadline(t, newStrictReader(src)); err == nil {
			t.Fatalf("accepted incomplete parameter: %q", src)
		}
	}
}

func parseZshWithDeadline(t *testing.T, r io.Reader) (*File, error) {
	t.Helper()
	// Normal parsing takes milliseconds. Bound repeated scanning and EOF
	// regressions so they fail without hanging the suite.
	type result struct {
		file *File
		err  error
	}
	done := make(chan result, 1)
	go func() {
		f, err := NewParser(Variant(LangZsh)).Parse(r, "")
		done <- result{f, err}
	}()
	select {
	case parsed := <-done:
		return parsed.file, parsed.err
	case <-time.After(5 * time.Second):
		t.Fatal("lookahead exceeded five seconds")
		return nil, nil
	}
}

func TestDollarParenReadingDialectGate(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangPOSIX, LangMirBSDKorn, LangBats} {
		if _, err := NewParser(Variant(lang)).Parse(strings.NewReader(`print $((print ok) | cat)`), ""); err == nil {
			t.Fatalf("%v: ambiguous command substitution must retain its error", lang)
		}
	}
}
