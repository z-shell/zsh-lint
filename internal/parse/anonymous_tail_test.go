package parse

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// Invocation tails stay outside the function body and outside its metadata
// words, at the original source positions (#279).
func TestAnonymousInvocationTails(t *testing.T) {
	tests := []struct {
		name, source, op string
		statements       int
		background       bool
		redirs           int
		comment          bool
	}{
		{name: "and", source: "() { print $1 } later && print x\n", op: "&&", statements: 1},
		{name: "or", source: "() { print $1 } later || print x\n", op: "||", statements: 1},
		{name: "pipe", source: "() { print $1 } later | cat\n", op: "|", statements: 1},
		{name: "pipe stderr", source: "() { print $1 } later |& cat\n", op: "|&", statements: 1},
		{name: "background", source: "() { print $1 } later & print x\n", statements: 2, background: true},
		{name: "redirect", source: "() { print $1 } later > out\n", statements: 1, redirs: 1},
		{name: "comment", source: "() { print $1 } later # retained\n", statements: 1, comment: true},
		{name: "group", source: "{ () { print $1 } later }\n", statements: 1},
		{name: "substitution", source: "print $( () { print $1 } later)\n", statements: 1},
		{name: "next line", source: "() { print $1 } later\nprint x\n", statements: 2},
		{name: "semicolon", source: "() { print $1 } later; print x\n", statements: 2},
		{name: "quoted operator", source: "() { print $1 } 'later' && print x\n", op: "&&", statements: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tt.source), "tail.zsh")
			if err != nil {
				t.Fatal(err)
			}
			if len(file.AST().Stmts) != tt.statements {
				t.Fatalf("statements = %d, want %d", len(file.AST().Stmts), tt.statements)
			}
			inv := file.AnonymousInvocations()
			if len(inv) != 1 || len(inv[0].Words) != 1 {
				t.Fatalf("invocations = %+v", inv)
			}
			word := inv[0].Words[0]
			start := strings.Index(tt.source, "later")
			end := start + len("later")
			if tt.name == "quoted operator" {
				start--
				end++
			}
			if int(word.Pos().Offset()) != start || int(word.End().Offset()) != end || word.Pos().Line() != 1 || int(word.Pos().Col()) != start+1 {
				t.Fatalf("word range = %v-%v (%d-%d), want %d-%d", word.Pos(), word.End(), word.Pos().Offset(), word.End().Offset(), start, end)
			}
			if tt.name != "quoted operator" && getParseWordLiteral(word) != "later" {
				t.Fatalf("word = %+v", word)
			}
			stmt := file.AST().Stmts[0]
			if tt.op != "" {
				binary, ok := stmt.Cmd.(*syntax.BinaryCmd)
				if !ok || binary.Op.String() != tt.op {
					t.Fatalf("operator = %T %+v", stmt.Cmd, stmt.Cmd)
				}
				if binary.X.Cmd != inv[0].Function || int(binary.OpPos.Offset()) != strings.Index(tt.source, tt.op) {
					t.Fatal("operator is not attached to the invocation at its original position")
				}
				if binary.Y == nil {
					t.Fatal("operator lost its right statement")
				}
				stmt = binary.X
			}
			if tt.background {
				if !stmt.Background || stmt.Cmd != inv[0].Function || getParseWordLiteral(file.AST().Stmts[1].Cmd.(*syntax.CallExpr).Args[1]) != "x" {
					t.Fatal("background flag or following command lost")
				}
			}
			if tt.redirs > 0 {
				if len(stmt.Redirs) != tt.redirs || stmt.Cmd != inv[0].Function || getParseWordLiteral(stmt.Redirs[0].Word) != "out" || int(stmt.Redirs[0].OpPos.Offset()) != strings.Index(tt.source, ">") {
					t.Fatal("redirect lost or moved")
				}
			}
			if inv[0].Function.Body.Cmd.(*syntax.Block).Rbrace.Offset() != uint(strings.Index(tt.source, "}")) {
				t.Fatal("function body includes the tail")
			}
			if tt.comment {
				comments := 0
				syntax.Walk(file.AST(), func(n syntax.Node) bool {
					if comment, ok := n.(*syntax.Comment); ok {
						comments++
						if int(comment.Hash.Offset()) != strings.Index(tt.source, "#") || comment.Text != " retained" {
							t.Fatal("comment changed")
						}
					}
					return true
				})
				if comments != 1 {
					t.Fatalf("comments = %d", comments)
				}
			}
		})
	}
}

func TestAnonymousInvocationTailWordBoundaries(t *testing.T) {
	for _, words := range []string{
		`'a && | }' second`, `a\&b second`, `"$value" second`,
		`$(case x in x) print x;; esac) second`, `${x::=value} second`,
		`"${x::=value}" second`, `${m[(r)a,b[^:]]} second`,
		`"${m[(r)a,b[^:]]}" second`, `${x::=one}${y::=two} second`,
		"first \\\nsecond", "'first\nword' second",
		`first > out second`, `first < in second`, `first 2>out second`,
		`first {fd}>out second`, "'first\n\nword' second",
	} {
		source := "() { :; } " + words + " && print tail\n"
		file, err := Parse(strings.NewReader(source), "words.zsh")
		if err != nil {
			t.Errorf("%q: %v", words, err)
			continue
		}
		inv := file.AnonymousInvocations()
		if len(inv) != 1 || len(inv[0].Words) != 2 || getParseWordLiteral(inv[0].Words[1]) != "second" {
			t.Fatalf("%q: invocation words = %+v", words, inv)
		}
		second := inv[0].Words[1]
		if int(second.Pos().Offset()) != strings.LastIndex(source, "second") {
			t.Fatalf("%q: second word moved", words)
		}
		binary := file.AST().Stmts[0].Cmd.(*syntax.BinaryCmd)
		offset := strings.LastIndex(source, "&&")
		line, col := lineCol([]byte(source), offset)
		if binary.OpPos.Offset() != uint(offset) || binary.OpPos.Line() != line || binary.OpPos.Col() != col {
			t.Fatalf("%q: operator position = %v, want %d:%d", words, binary.OpPos, line, col)
		}
	}
}

func TestAnonymousInvocationTailRedirectPrefixes(t *testing.T) {
	for _, redirect := range []string{"2>/dev/null", "2>&1", "{fd}>out"} {
		source := "() { :; } argument " + redirect + "\n"
		file, err := Parse(strings.NewReader(source), "redirect.zsh")
		if err != nil {
			t.Fatal(err)
		}
		inv := file.AnonymousInvocations()
		if len(inv) != 1 || len(inv[0].Words) != 1 || getParseWordLiteral(inv[0].Words[0]) != "argument" {
			t.Fatalf("%q: redirect prefix entered invocation words: %+v", redirect, inv)
		}
		redirs := file.AST().Stmts[0].Redirs
		if len(redirs) != 1 || redirs[0].N == nil || int(redirs[0].N.Pos().Offset()) != strings.Index(source, redirect) {
			t.Fatalf("%q: redirect descriptor lost or moved: %+v", redirect, redirs)
		}
	}
}

func TestAnonymousInvocationTailInvalid(t *testing.T) {
	for _, source := range []string{
		"() { :; } arg}\n", "() { :; } arg >\n", "() { :; } arg | |\n",
		"() { :; } arg \"unterminated\n", "named() { :; } arg && :\n", "{ :; } arg && :\n",
	} {
		if _, err := Parse(strings.NewReader(source), "invalid.zsh"); err == nil {
			t.Errorf("invalid source accepted: %q", source)
		}
	}
}
