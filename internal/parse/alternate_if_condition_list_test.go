package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The alternate forms take a condition *list* (#376): a plain command may head
// an `&&`/`||` or pipeline sublist, and the brace after the last element is the
// body. Each row passes `zsh -f -n`.
func TestAlternateConditionListAccepted(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"and test", "if true && [[ -z $x ]] { print a }\n"},
		{"and arithmetic", "if true && (( 1 )) { print a }\n"},
		{"or test", "if true || [[ -z $x ]] { print a }\n"},
		{"command with args and else", "if f arg && [[ -z $x ]] { print a } else { print b }\n"},
		{"while", "while true && [[ -z $x ]] { print a }\n"},
		{"elif", "if [[ z ]] { : } elif f && [[ -z $x ]] { print a }\n"},
		{"elif then else", "if f && [[ a ]] { : } elif g && [[ b ]] { : } else { : }\n"},
		{"pipeline head", "if print x | read -r y && [[ -n $y ]] { print a }\n"},
		{"arithmetic chain", "if (( 1 )) && true && (( 1 )) { print a }\n"},
		{"condition group then body", "if f && { true } { print a }\n"},
		{"test chain", "if [[ a ]] && true && [[ b ]] { print a }\n"},
		{"multiline body", "if f && [[ -z $x ]] {\n  print a\n}\nprint after\n"},
		{"trailing separator", "if f && [[ a ]] { print a }; print after\n"},
		{"trailing comment", "if f && [[ a ]] { print a } # note\n"},
		{"in function", "g() {\n  if f && [[ a ]] { return 1 }\n}\n"},
		{"corpus zi install.zsh:396", "if .zi-get-object-path plugin \"$id_as\" && [[ -z $update ]] {\n  return 1\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// Sources native Zsh rejects. The retry frames its synthetic `fi` and `done`
// with newlines, so a tail glued to the closing brace would be detached and
// accepted; the new shapes must not gain that.
func TestAlternateConditionListRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"glued group body", "if f && { true }{ print a }\n"},
		{"group without body", "if f && { true }\n"},
		{"word after body", "if f && [[ a ]] { : } print z\n"},
		{"subshell after body", "if f && [[ a ]] { : } ( : )\n"},
		{"glued comment", "if f && [[ a ]] { : }#c\n"},
		{"double semicolon", "if f && [[ a ]] { : } ;;\n"},
		{"else after else", "if f && [[ a ]] { : } else { : } else { : }\n"},
		{"stray fi", "if f && [[ a ]] { : } fi\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// A rejected list-condition construct must not take the rest of the file
// down with it: the error still points at the construct, not at an earlier
// valid alternate form that the scan would otherwise stop rewriting.
func TestAlternateConditionListRejectionIsLocal(t *testing.T) {
	src := "if [[ z ]] { : }\nif f && [[ a ]] { : } print z\n"
	_, err := parseWithAdapters([]byte(src), "t.zsh")
	if err == nil {
		t.Fatalf("invalid Zsh accepted: %q", src)
	}
	if !strings.Contains(err.Error(), ":2:") {
		t.Fatalf("error %q does not point at line 2", err)
	}
}

// Parsing is not enough: the brace must become the body, and the whole list
// must stay in the condition.
func TestAlternateConditionListShape(t *testing.T) {
	src := "if f arg && [[ -z $x ]] { print BODY } else { print ELSE }\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}
	var clause *syntax.IfClause
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if found, ok := node.(*syntax.IfClause); ok && clause == nil {
			clause = found
		}
		return true
	})
	if clause == nil {
		t.Fatal("no if clause")
	}
	if len(clause.Cond) != 1 {
		t.Fatalf("condition holds %d statements, want 1", len(clause.Cond))
	}
	binary, ok := clause.Cond[0].Cmd.(*syntax.BinaryCmd)
	if !ok || binary.Op != syntax.AndStmt {
		t.Fatalf("condition is %T, want && BinaryCmd", clause.Cond[0].Cmd)
	}
	if _, ok := binary.X.Cmd.(*syntax.CallExpr); !ok {
		t.Errorf("condition head is %T, want *syntax.CallExpr", binary.X.Cmd)
	}
	if _, ok := binary.Y.Cmd.(*syntax.TestClause); !ok {
		t.Errorf("condition tail is %T, want *syntax.TestClause", binary.Y.Cmd)
	}
	for name, stmts := range map[string][]*syntax.Stmt{"then": clause.Then, "else": clause.Else.Then} {
		if len(stmts) != 1 {
			t.Fatalf("%s holds %d statements, want 1", name, len(stmts))
		}
	}
	if got := src[clause.Then[0].Pos().Offset():clause.Then[0].End().Offset()]; got != "print BODY" {
		t.Errorf("body is %q, want %q", got, "print BODY")
	}
}

// while gets the same treatment: the brace is the loop body.
func TestAlternateWhileConditionListShape(t *testing.T) {
	src := "while true && [[ -z $x ]] { print BODY }\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}
	var loop *syntax.WhileClause
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if found, ok := node.(*syntax.WhileClause); ok && loop == nil {
			loop = found
		}
		return true
	})
	if loop == nil {
		t.Fatal("no while clause")
	}
	if len(loop.Cond) != 1 || len(loop.Do) != 1 {
		t.Fatalf("cond %d / do %d statements, want 1 / 1", len(loop.Cond), len(loop.Do))
	}
	if got := src[loop.Do[0].Pos().Offset():loop.Do[0].End().Offset()]; got != "print BODY" {
		t.Errorf("body is %q, want %q", got, "print BODY")
	}
}
