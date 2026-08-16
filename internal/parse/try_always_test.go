package parse

import (
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseTryAlwaysAcceptsNativeSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "top level",
			src:  "{\n  print try\n} always {\n  print cleanup\n}\n",
		},
		{
			name: "tab separated boundary",
			src:  "{\n  print try\n}\talways\t{\n  print cleanup\n}\n",
		},
		{
			name: "nested in function",
			src:  "cleanup() {\n  {\n    print try\n  } always {\n    print cleanup\n  }\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(file.AST().Stmts) == 0 {
				t.Fatal("Parse() returned no statements")
			}
		})
	}
}

func TestParseTryAlwaysRejectsInvalidSeparators(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "newline before always",
			src:  "{\n  print try\n}\nalways {\n  print cleanup\n}\n",
		},
		{
			name: "semicolon before always",
			src:  "{\n  print try\n}; always {\n  print cleanup\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err == nil {
				t.Fatal("Parse() succeeded for native-invalid try/always separator")
			}
		})
	}
}

func TestParseTryAlwaysPreservesHeredocBody(t *testing.T) {
	src := "cat <<'EOF'\n} always {\nEOF\n{\n  print try\n} always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "heredoc.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	var rendered strings.Builder
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("printing AST: %v", err)
	}
	if !strings.Contains(rendered.String(), "} always {") {
		t.Fatalf("rendered heredoc body = %q, want original try/always text", rendered.String())
	}
}

func TestParseTryAlwaysPreservesInactiveLookalikes(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "comment", prefix: "# } always {\n"},
		{name: "single quoted word", prefix: "print '} always {'\n"},
		{name: "command substitution", prefix: "value=$(print '} always {')\n"},
		{name: "legacy backtick substitution", prefix: "value=`print '} always {'`\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := test.prefix + "{\n  print try\n} always {\n  print cleanup\n}\n"
			file, err := Parse(strings.NewReader(src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var rendered strings.Builder
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("printing AST: %v", err)
			}
			if !strings.Contains(rendered.String(), "} always {") {
				t.Fatalf("rendered source = %q, want inactive lookalike preserved", rendered.String())
			}
		})
	}
}

func TestParseTryAlwaysAfterArithmeticShift(t *testing.T) {
	src := "(( mask = 1 << 2 ))\n{\n  print try\n} always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "arithmetic-shift.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(file.AST().Stmts) != 3 {
		t.Fatalf("statement count = %d, want 3", len(file.AST().Stmts))
	}
}

func TestParseTryAlwaysPreservesBodyPositions(t *testing.T) {
	src := "{\n  print try\n}  always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	var positions []syntax.Pos
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || call.Args[0].Lit() != "print" {
			return true
		}
		positions = append(positions, call.Pos())
		return true
	})
	if len(positions) != 2 {
		t.Fatalf("print positions = %v, want two", positions)
	}
	for i, want := range []struct{ line, col uint }{{2, 3}, {4, 3}} {
		if positions[i].Line() != want.line || positions[i].Col() != want.col {
			t.Errorf("print[%d].Pos() = %d:%d, want %d:%d", i, positions[i].Line(), positions[i].Col(), want.line, want.col)
		}
	}
}

func TestParseTryAlwaysRetriesOnce(t *testing.T) {
	src := []byte("{\n  :\n} always {\n  :\n}\n")
	_, firstErr := parseTree(src, "retry-once.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the original source")
	}

	attempts := 0
	_, err := parseTryAlwaysWithParser(src, "retry-once.zsh", firstErr, func(transformed []byte, name string) (*syntax.File, error) {
		attempts++
		if len(transformed) != len(src) {
			t.Fatalf("transformed length = %d, want %d", len(transformed), len(src))
		}
		return parseTree(transformed, name)
	})
	if err != nil {
		t.Fatalf("parseTryAlwaysWithParser() error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("compatibility parser attempts = %d, want 1", attempts)
	}

	_, err = parseTryAlwaysWithParser(src, "wrong-error.zsh", errors.New("other parse error"), parseTree)
	if err == nil {
		t.Fatal("parseTryAlwaysWithParser() unexpectedly accepted a non-parser error")
	}
}

func TestScanTryAlwaysEditsRequiresErrorAtBoundary(t *testing.T) {
	src := []byte("{\n  :\n} always {\n  :\n}\n")
	if edits, ok := scanTryAlwaysEdits(src, 0); ok || len(edits) != 1 {
		t.Fatalf("scanTryAlwaysEdits() = (%v, %t), want one edit and false", edits, ok)
	}
}
