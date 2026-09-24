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
		{"negated group", "if f && ! { g } { print a }\n"},
		{"timed group", "if f && time { g } { print a }\n"},
		{"group then test", "if f && { g } && [[ a ]] { print a }\n"},
		{"clobber redirect head", "if f >| o && [[ a ]] { print a }\n"},
		{"subshell head", "if (f) && [[ a ]] { print a }\n"},
		{"connective ends line", "if f &&\n[[ a ]] { print a }\n"},
		{"pipe ends line", "if f |\n[[ a ]] { print a }\n"},
		{"nested body on next line", "if [[ x ]] { if f && [[ c ]] { : }\n}\n"},
		{"quoted brace in group", "if f && { print '}' } { print a }\n"},
		{"quoted open brace in group", "if f && { print \"{\" } { print a }\n"},
		{"alternate form inside group", "if f && { if g && [[ a ]] { h }\n} { print a }\n"},
		// Connectives inside an if body must not mark a later plain else.
		{"connective in body, else redirect", "if [[ z ]] { while f && g; do :; done } else { : } > /dev/null\n"},
		{"connective in body, else in function", "h() { if [[ z ]] { if f || g; then :; fi } else { : } }\n"},
		{"pipe in body, else redirect", "if [[ z ]] { f | g; while f | g; do :; done } else { : } 2>&1\n"},
		{"connective in elif body", "if [[ a ]] { : } elif [[ b ]] { if f && g; then :; fi } else { : } > /dev/null\n"},
		// Braces in words after a connective are arguments, not groups.
		{"braced words in classic if", "if f && print a{b} {c}; then :; fi\nif [[ z ]] { : }\n"},
		{"brace expansion in classic if", "if [[ z ]] { : }\nif f && mkdir -p d/{a,b} {c,d}; then :; fi\n"},
		{"braced words in classic while", "if [[ z ]] { : }\nwhile f || print a{b} {c}; do break; done\n"},
		{"pipe in command substitution", "if [[ z ]] { : }\nif print $(f | read) {x} {y}; then :; fi\n"},
		{"and in backquotes", "if [[ z ]] { : }\nif print `f && g` {x} {y}; then :; fi\n"},
		{"glob alternation", "if [[ z ]] { : }\nif ls (a|b) {c} {d}; then :; fi\n"},
		// A quoted brace in a classic condition group must not desync quoting.
		{"quoted close brace in classic group", "if f && { print '}' }; then :; fi\nif [[ z ]] { : }\n"},
		{"double-quoted close brace in classic group", "while f || { print \"}\"; }; do break; done\nif [[ z ]] { : }\n"},
		{"alternate if inside classic group", "if f && {\n  if [[ a ]] { g }\n}; then :; fi\n"},
		{"alternate while inside classic group", "if f && {\n  while [[ a ]] { break }\n}; then :; fi\n"},
		// Elements separated by `;` or a newline keep main's verdicts.
		{"semicolon-separated tests", "if [[ a ]]; [[ b ]] { print a }\n"},
		{"newline-separated tests", "if [[ a ]]\n! [[ b ]] { print a }\n"},
		{"semicolon then group", "if [[ a ]]; { g } { print a }\n"},
		{"elif after test-like argument", "if false; then :; elif print [[ a ]]\n(( 1 )) { print a }\n"},
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
		{"glued body after group", "if f && { g }{ print B }\n"},
		{"anonymous function after connective", "if f && () { g } { print B }\n"},
		{"function keyword after connective", "if f && function { g } { print B }\n"},
		{"while anonymous function after connective", "while f && () { g } { print B; break }\n"},
		{"brace-expansion word as body", "if [[ z ]] { : }\nif f && print {a,b} { print B }\n"},
		{"braced word as body", "if [[ z ]] { : }\nif f && print x{a} { print B }\n"},
		// The inner construct's tail is the outer closer on the same line.
		// The retry must not give it the synthetic `fi` line as a tail.
		{"nested in if, outer closer tail", "if [[ x ]] { if f && [[ c ]] { : } }\n"},
		{"nested in while, outer closer tail", "while [[ x ]] { if f && [[ c ]] { : } }\n"},
		{"nested in list-condition if", "if f && [[ x ]] { if g && [[ c ]] { : } }\n"},
		{"nested in else", "if [[ a ]] { : } elif [[ b ]] { : } else { if f && [[ c ]] { : } }\n"},
		{"nested in group, group closer tail", "if f && { if g && [[ a ]] { h } } { print a }\n"},
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
