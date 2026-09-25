package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// assignExpansions returns the `:=`-shaped expansions in source order, which
// after the adapter includes every `::=` expansion.
func assignExpansions(tree *syntax.File) []*syntax.ParamExp {
	var found []*syntax.ParamExp
	syntax.Walk(tree, func(node syntax.Node) bool {
		if exp, ok := node.(*syntax.ParamExp); ok && exp.Exp != nil && exp.Exp.Op == syntax.AssignUnsetOrNull {
			found = append(found, exp)
		}
		return true
	})
	return found
}

func wordText(t *testing.T, word *syntax.Word) string {
	t.Helper()
	if word == nil {
		return ""
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, word); err != nil {
		t.Fatalf("print word: %v", err)
	}
	return rendered.String()
}

// Issue #216: `${name::=word}` assigns unconditionally. mvdan/sh reads the
// `::` as a slice and fails on `=word`; the adapter must accept the operator,
// keep the assigned word's text and position, and report the expansion as
// File metadata without changing `${name:=word}`.
func TestParseAssignAlways(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantParam string
		wantWord  string
		wantPos   string
		wantParts int
	}{
		{"literal word", "print ${x::=value}\n", "x", "value", "1:13", 1},
		{"expansion word", "print ${x::=$y}\n", "x", "$y", "1:13", 1},
		{"quoted expansion word", "v=${name::=\"${(kv)a[@]}\"}\n", "name", "\"${(kv)a[@]}\"", "1:12", 1},
		{"empty word", "print ${x::=}\n", "x", "", "", 0},
		{"mixed word", "print ${x::=a$y}\n", "x", "a$y", "1:13", 2},
		{"word with space", "print ${x::= v}\n", "x", " v", "1:13", 1},
		{"subscripted name", "print ${arr[1]::=first}\n", "arr", "first", "1:18", 1},
		{"flagged name", ": ${(PA)n::=\"${(kv)a[@]}\"}\n", "n", "\"${(kv)a[@]}\"", "1:13", 1},
		{"inside double quotes", "print \"${x::=value}\"\n", "x", "value", "1:14", 1},
		{"nested in conditional", "print ${y:+${x::=$y}}\n", "x", "$y", "1:18", 1},
		{"nested in modifier", "print ${${x::=inner}#in}\n", "x", "inner", "1:15", 1},
		{"second line", "print hi\nprint ${x::=value}\n", "x", "value", "2:13", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			always := file.AssignAlwaysExpansions()
			if len(always) != 1 {
				t.Fatalf("AssignAlwaysExpansions() = %d expansions, want 1", len(always))
			}
			exp := always[0]
			if exp.Param == nil || exp.Param.Value != test.wantParam {
				t.Fatalf("Param = %v, want %q", exp.Param, test.wantParam)
			}
			if exp.Exp == nil || exp.Exp.Op != syntax.AssignUnsetOrNull {
				t.Fatalf("Exp = %v, want the := operator shape", exp.Exp)
			}
			if got := wordText(t, exp.Exp.Word); got != test.wantWord {
				t.Fatalf("assigned word = %q, want %q", got, test.wantWord)
			}
			if test.wantParts == 0 {
				if exp.Exp.Word != nil {
					t.Fatalf("assigned word = %v, want nil for an empty word", exp.Exp.Word)
				}
				return
			}
			if got := len(exp.Exp.Word.Parts); got != test.wantParts {
				t.Fatalf("word parts = %d, want %d", got, test.wantParts)
			}
			if got := exp.Exp.Word.Pos().String(); got != test.wantPos {
				t.Fatalf("word position = %s, want %s", got, test.wantPos)
			}
			// Every part must map onto the original bytes at its position.
			for _, part := range exp.Exp.Word.Parts {
				start, end := int(part.Pos().Offset()), int(part.End().Offset())
				if lit, ok := part.(*syntax.Lit); ok && test.src[start:end] != lit.Value {
					t.Fatalf("literal %q at %s does not match source %q", lit.Value, lit.Pos(), test.src[start:end])
				}
			}
		})
	}
}

func TestParseAssignAlwaysPreservesSurroundingTree(t *testing.T) {
	src := "print ${x::=value} ${y:=cond} ${z::=} after\n"
	file, err := Parse(strings.NewReader(src), "surrounding.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	assigns := assignExpansions(file.AST())
	if len(assigns) != 3 {
		t.Fatalf("found %d := expansions, want 3", len(assigns))
	}
	always := file.AssignAlwaysExpansions()
	if len(always) != 2 || always[0] != assigns[0] || always[1] != assigns[2] {
		t.Fatalf("AssignAlwaysExpansions() = %v, want the first and third expansions", always)
	}
	if got := wordText(t, assigns[1].Exp.Word); got != "cond" {
		t.Fatalf("conditional word = %q, want %q", got, "cond")
	}
	call := file.AST().Stmts[0].Cmd.(*syntax.CallExpr)
	if len(call.Args) != 5 {
		t.Fatalf("call has %d args, want 5", len(call.Args))
	}
	if got := call.Args[4].Pos().String(); got != "1:39" {
		t.Fatalf("trailing word position = %s, want 1:39", got)
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print AST: %v", err)
	}
	if got, want := rendered.String(), "print ${x:=value} ${y:=cond} ${z:=} after\n"; got != want {
		t.Fatalf("rendered tree = %q, want the := shape %q", got, want)
	}
}

func TestParseAssignAlwaysLeavesConditionalUnchanged(t *testing.T) {
	src := "print ${x:=value} ${x:==literal}\n"
	file, err := Parse(strings.NewReader(src), "conditional.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := file.AssignAlwaysExpansions(); len(got) != 0 {
		t.Fatalf("AssignAlwaysExpansions() = %v, want none", got)
	}
	assigns := assignExpansions(file.AST())
	if len(assigns) != 2 {
		t.Fatalf("found %d := expansions, want 2", len(assigns))
	}
	if got := wordText(t, assigns[1].Exp.Word); got != "=literal" {
		t.Fatalf("conditional word = %q, want %q (the leading = is the user's)", got, "=literal")
	}
}

func TestParseAssignAlwaysRejectsInvalidSources(t *testing.T) {
	for _, fixture := range []string{
		"testdata/invalid-216-empty-name-assign-always.txt",
		"testdata/invalid-216-triple-colon-assign-always.txt",
	} {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-216.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) || parseErr.Pos.Line() != 2 {
				t.Fatalf("Parse() error = %v, want a parse error on line 2", err)
			}
		})
	}
}

// The adapter must not consume the gate error for an unrelated construct: a
// bare arithmetic assignment without a name reports the same text.
func TestParseAssignAlwaysIgnoresUnrelatedGateError(t *testing.T) {
	src := "(( = 1 ))\n"
	_, err := Parse(strings.NewReader(src), "arith.zsh")
	if err == nil {
		t.Fatal("Parse() accepted an arithmetic assignment without a name")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != invalidAssignAlwaysOperator {
		t.Fatalf("Parse() error = %v, want the original %q", err, invalidAssignAlwaysOperator)
	}
}

// A retry that parses but produces no matching expansion must hand back the
// original error rather than an unverified tree.
func TestParseAssignAlwaysRequiresRestoredWord(t *testing.T) {
	src := []byte("print ${x::=value}\n")
	firstErr := syntax.ParseError{Pos: syntax.NewPos(11, 1, 12), Text: invalidAssignAlwaysOperator}
	_, err := parseAssignAlwaysWithParser(src, "stub.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		return &syntax.File{Name: "stub.zsh"}, nil
	})
	if !errors.Is(err, firstErr) {
		t.Fatalf("error = %v, want the original parser error", err)
	}
}
