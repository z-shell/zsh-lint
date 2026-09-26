package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestForeachTreeShape(t *testing.T) {
	t.Parallel()
	src := "foreach item (alpha beta)\n  print -r -- $item\nend\n"
	file, err := Parse(strings.NewReader(src), "foreach.zsh")
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if len(file.AST().Stmts) != 1 {
		t.Fatalf("len(Stmts) = %d, want 1", len(file.AST().Stmts))
	}
	fc, ok := file.AST().Stmts[0].Cmd.(*syntax.ForClause)
	if !ok {
		t.Fatalf("Cmd is %T, want *syntax.ForClause", file.AST().Stmts[0].Cmd)
	}

	if got, want := int(fc.Pos().Offset()), strings.Index(src, "foreach"); got != want {
		t.Errorf("fc.Pos().Offset() = %d, want %d", got, want)
	}
	if got, want := int(fc.DonePos.Offset()), strings.Index(src, "end"); got != want {
		t.Errorf("fc.DonePos.Offset() = %d, want %d", got, want)
	}
	if got, want := int(fc.End().Offset()), strings.Index(src, "end")+len("end"); got != want {
		t.Errorf("fc.End().Offset() = %d, want %d", got, want)
	}

	iter, ok := fc.Loop.(*syntax.WordIter)
	if !ok {
		t.Fatalf("fc.Loop is %T, want *syntax.WordIter", fc.Loop)
	}
	if iter.Name == nil || iter.Name.Value != "item" {
		t.Errorf("iter.Name = %v, want 'item'", iter.Name)
	}
	if got, want := int(iter.Name.Pos().Offset()), strings.Index(src, "item"); got != want {
		t.Errorf("iter.Name.Pos().Offset() = %d, want %d", got, want)
	}

	if len(iter.Items) != 2 {
		t.Fatalf("len(iter.Items) = %d, want 2", len(iter.Items))
	}
	if got, want := int(iter.Items[0].Pos().Offset()), strings.Index(src, "alpha"); got != want {
		t.Errorf("iter.Items[0].Pos().Offset() = %d, want %d", got, want)
	}
	if got, want := int(iter.Items[1].Pos().Offset()), strings.Index(src, "beta"); got != want {
		t.Errorf("iter.Items[1].Pos().Offset() = %d, want %d", got, want)
	}

	if len(fc.Do) != 1 {
		t.Fatalf("len(fc.Do) = %d, want 1", len(fc.Do))
	}
	if got, want := int(fc.DoPos.Offset()), strings.Index(src, "print"); got != want {
		t.Errorf("fc.DoPos.Offset() = %d, want %d", got, want)
	}
	if got, want := int(fc.Do[0].Pos().Offset()), strings.Index(src, "print"); got != want {
		t.Errorf("fc.Do[0].Pos().Offset() = %d, want %d", got, want)
	}
}

func TestForeachMultipleNames(t *testing.T) {
	t.Parallel()
	src := "foreach a b (1 2 3 4)\n  print $a $b\nend\n"
	file, err := Parse(strings.NewReader(src), "foreach_multi.zsh")
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	fc, ok := file.AST().Stmts[0].Cmd.(*syntax.ForClause)
	if !ok {
		t.Fatalf("Cmd is %T, want *syntax.ForClause", file.AST().Stmts[0].Cmd)
	}
	iter, ok := fc.Loop.(*syntax.WordIter)
	if !ok {
		t.Fatalf("fc.Loop is %T, want *syntax.WordIter", fc.Loop)
	}
	if iter.Name == nil || iter.Name.Value != "a" {
		t.Errorf("iter.Name = %v, want 'a'", iter.Name)
	}
	if len(iter.Items) != 4 {
		t.Fatalf("len(iter.Items) = %d, want 4", len(iter.Items))
	}
	for i, want := range []string{"1", "2", "3", "4"} {
		if iter.Items[i].Lit() != want {
			t.Errorf("iter.Items[%d] = %q, want %q", i, iter.Items[i].Lit(), want)
		}
	}
}

func TestForeachVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{"in list with end", "foreach v in a b; print $v; end\n"},
		{"brace body", "foreach v (a b) { print $v }\n"},
		{"do done body", "foreach v (a b) do print $v; done\n"},
		{"empty body newline", "foreach v (a b)\nend\n"},
		{"empty body semicolon", "foreach v (a b); end\n"},
		{"positional parameters", "foreach v; print $v; end\n"},
		{"nested foreach", "foreach x (1 2)\n  foreach y (3 4)\n    print $x $y\n  end\nend\n"},
		{"foreach inside function", "foo() {\n  foreach v (a b)\n    print $v\n  end\n}\n"},
		{"end as argument inside body", "foreach v (a b)\n  print end\nend\n"},
		{"end followed by semicolon", "foreach v (a b)\n  print $v\nend; print done\n"},
		{"end followed by pipe", "foreach v (a b)\n  print $v\nend | cat\n"},
		{"end followed by and", "foreach v (a b)\n  print $v\nend && print ok\n"},
		{"end followed by redirect", "foreach v (a b)\n  print $v\nend > /dev/null\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), tc.name+".zsh")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			var found []*syntax.ForClause
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if fc, ok := node.(*syntax.ForClause); ok {
					found = append(found, fc)
				}
				return true
			})
			if len(found) == 0 {
				t.Fatalf("no ForClause found in tree for %q", tc.src)
			}
		})
	}
}

func TestForeachInvalidSyntax(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{"lone end", "end\n"},
		{"lone foreach", "foreach\n"},
		{"no end on one line", "foreach v (a b); print $v\n"},
		{"no end multi-line", "foreach v (a b)\n  print $v\n"},
		{"mismatched do with end", "foreach v (a b) do print $v end\n"},
		{"mismatched foreach with done", "foreach v (a b)\n  print $v\ndone\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.src), tc.name+".zsh")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want syntax error", tc.src)
			}
			var perr syntax.ParseError
			if !errorsAs(err, &perr) {
				t.Fatalf("error is %T (%v), want syntax.ParseError", err, err)
			}
		})
	}
}

func errorsAs(err error, target any) bool {
	type causer interface {
		As(any) bool
	}
	if c, ok := err.(causer); ok {
		return c.As(target)
	}
	// Fall back to standard type check.
	switch t := target.(type) {
	case *syntax.ParseError:
		if pe, ok := err.(syntax.ParseError); ok {
			*t = pe
			return true
		}
	}
	return false
}

func TestForeachInvalidFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		"testdata/invalid-214-no-end.txt",
		"testdata/invalid-214-lone-end.txt",
		"testdata/invalid-214-lone-foreach.txt",
	}
	for _, fix := range fixtures {
		t.Run(filepath.Base(fix), func(t *testing.T) {
			data, err := os.ReadFile(fix)
			if err != nil {
				t.Fatalf("ReadFile(%s) failed: %v", fix, err)
			}
			_, err = Parse(strings.NewReader(string(data)), fix)
			if err == nil {
				t.Fatalf("Parse(%s) succeeded, want syntax error", fix)
			}
			var perr syntax.ParseError
			if !errorsAs(err, &perr) {
				t.Fatalf("error is %T (%v), want syntax.ParseError", err, err)
			}
		})
	}
}
