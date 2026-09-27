package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestFunctionKeywordEmptyBodyTreeShape(t *testing.T) {
	t.Parallel()

	// Assert Body == nil for "function a"
	{
		file, err := Parse(strings.NewReader("function a\n"), "test-empty.zsh")
		if err != nil {
			t.Fatalf("Parse(\"function a\") failed: %v", err)
		}
		if len(file.AST().Stmts) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(file.AST().Stmts))
		}
		fn, ok := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
		if !ok {
			t.Fatalf("expected *syntax.FuncDecl, got %T", file.AST().Stmts[0].Cmd)
		}
		if fn.Body != nil {
			t.Errorf("expected fn.Body == nil, got %v", fn.Body)
		}
		if fn.Name == nil || fn.Name.Value != "a" {
			t.Errorf("expected fn.Name 'a', got %v", fn.Name)
		}
		if fn.End() != fn.Name.End() {
			t.Errorf("expected fn.End() == %v, got %v", fn.Name.End(), fn.End())
		}
	}

	// Assert non-nil nested FuncDecl body for "function a; function b; print z"
	{
		file, err := Parse(strings.NewReader("function a; function b; print z\n"), "test-nested.zsh")
		if err != nil {
			t.Fatalf("Parse(\"function a; function b; print z\") failed: %v", err)
		}
		if len(file.AST().Stmts) != 1 {
			t.Fatalf("expected 1 outer statement, got %d", len(file.AST().Stmts))
		}
		fnA, ok := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
		if !ok {
			t.Fatalf("expected outer *syntax.FuncDecl, got %T", file.AST().Stmts[0].Cmd)
		}
		if fnA.Name == nil || fnA.Name.Value != "a" {
			t.Errorf("expected outer func name 'a', got %v", fnA.Name)
		}
		if fnA.Body == nil {
			t.Fatalf("expected outer fnA.Body != nil, got nil")
		}
		fnB, ok := fnA.Body.Cmd.(*syntax.FuncDecl)
		if !ok {
			t.Fatalf("expected fnA.Body.Cmd to be inner *syntax.FuncDecl, got %T", fnA.Body.Cmd)
		}
		if fnB.Name == nil || fnB.Name.Value != "b" {
			t.Errorf("expected inner func name 'b', got %v", fnB.Name)
		}
		if fnB.Body == nil {
			t.Fatalf("expected inner fnB.Body != nil, got nil")
		}
		call, ok := fnB.Body.Cmd.(*syntax.CallExpr)
		if !ok {
			t.Fatalf("expected inner fnB.Body.Cmd to be *syntax.CallExpr, got %T", fnB.Body.Cmd)
		}
		if len(call.Args) < 2 || call.Args[0].Lit() != "print" || call.Args[1].Lit() != "z" {
			t.Errorf("expected 'print z', got %v", call.Args)
		}
	}
}

func TestFunctionKeywordEmptyBodyInvalidFixture(t *testing.T) {
	t.Parallel()

	fixturePath := "testdata/invalid-479-function-empty-body-double-semicolon.txt"
	src, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}

	_, parseErr := Parse(bytes.NewReader(src), fixturePath)
	if parseErr == nil {
		t.Fatalf("Parse(%s) unexpectedly succeeded", fixturePath)
	}

	var perr syntax.ParseError
	if !errors.As(parseErr, &perr) {
		t.Fatalf("expected syntax.ParseError, got %T: %v", parseErr, parseErr)
	}

	if perr.Pos.Line() != 2 || perr.Pos.Col() != 1 {
		t.Errorf("error pos = %v, want 2:1", perr.Pos)
	}
	wantText := "`foo()` must be followed by a statement"
	if perr.Text != wantText {
		t.Errorf("error text = %q, want %q", perr.Text, wantText)
	}
}
