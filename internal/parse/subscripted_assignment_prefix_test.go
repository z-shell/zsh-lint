package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A subscripted assignment before a command word is an ordinary Zsh prefix
// assignment (#285). The public parse path must keep the assignment, its
// subscript and the command word, including a pattern subscript, which the
// flag-subscript adapter reads. Each row is valid under `zsh -f -n`.
func TestParseSubscriptedAssignmentPrefix(t *testing.T) {
	for _, tc := range []struct {
		src   string
		name  string
		index string
		arg   string
	}{
		{"m[1]=1 true\n", "m", "1", "true"},
		{"m[(r)a,[^:]##]=1 x\n", "m", "(r)a,[^:]##", "x"},
		{"h[(i)k*]=v print\n", "h", "(i)k*", "print"},
		{"a[2]=x b=1 true\n", "a", "2", "true"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			stmts := file.AST().Stmts
			if len(stmts) != 1 {
				t.Fatalf("%d statements, want 1", len(stmts))
			}
			call, ok := stmts[0].Cmd.(*syntax.CallExpr)
			if !ok {
				t.Fatalf("command is %T, want *syntax.CallExpr", stmts[0].Cmd)
			}
			if len(call.Assigns) == 0 || len(call.Args) == 0 {
				t.Fatalf("%d assignments and %d words, want both", len(call.Assigns), len(call.Args))
			}
			first := call.Assigns[0]
			if first.Name == nil || first.Name.Value != tc.name {
				t.Errorf("assignment name %v, want %q", first.Name, tc.name)
			}
			if first.Index == nil {
				t.Fatal("assignment lost its subscript")
			}
			if got := tc.src[first.Index.Pos().Offset():first.Index.End().Offset()]; got != tc.index {
				t.Errorf("subscript %q, want %q", got, tc.index)
			}
			arg := call.Args[0]
			if got := tc.src[arg.Pos().Offset():arg.End().Offset()]; got != tc.arg {
				t.Errorf("command word %q, want %q", got, tc.arg)
			}
		})
	}
}
