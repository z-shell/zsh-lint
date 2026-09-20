package parse

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// ifBranches describes every `if` branch in the tree in source order as
// `kind keyword:firstStmt/count`, where kind is then, elif or else, keyword
// the position of the branch's `then` (or of `else`), firstStmt the position
// of its first statement (`-` for an empty branch) and count the number of
// statements.
func ifBranches(tree *syntax.File) []string {
	var found []string
	syntax.Walk(tree, func(node syntax.Node) bool {
		clause, ok := node.(*syntax.IfClause)
		if !ok {
			return true
		}
		kind, keyword := "then", clause.ThenPos
		switch {
		case !clause.ThenPos.IsValid():
			kind, keyword = "else", clause.Position
		case isElif(tree, clause):
			kind = "elif"
		}
		first := "-"
		if len(clause.Then) > 0 {
			first = clause.Then[0].Pos().String()
		}
		found = append(found, fmt.Sprintf("%s %s:%s/%d", kind, keyword, first, len(clause.Then)))
		return true
	})
	return found
}

// isElif reports whether clause hangs off another clause's Else, which is
// how the parser represents `elif`.
func isElif(tree *syntax.File, clause *syntax.IfClause) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if parent, ok := node.(*syntax.IfClause); ok && parent.Else == clause {
			found = true
		}
		return !found
	})
	return found
}

// Issue #297: an `if` branch may begin with a separator directly after
// `then` or `else`. Native Zsh reads the leading `;` as an empty sublist; the
// parser reads it as the whole branch and then demands `fi`. The adapter
// must accept the then, elif and else branches, keep every position, and
// leave the branch statements where the source has them.
func TestParseThenLeadingSeparator(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		branches []string
	}{
		{"then", "if true; then; print x; fi\n", []string{"then 1:10:1:16/1"}},
		{"else", "if true; then x; else; y; fi\n", []string{"then 1:10:1:15/1", "else 1:18:1:24/1"}},
		{"elif", "if true; then x; elif false; then; y; fi\n", []string{"then 1:10:1:15/1", "elif 1:30:1:36/1"}},
		{"then and else", "if true; then; x; else; y; fi\n", []string{"then 1:10:1:16/1", "else 1:19:1:25/1"}},
		{"all three", "if true; then; x; elif false; then; y; else; z; fi\n",
			[]string{"then 1:10:1:16/1", "elif 1:31:1:37/1", "else 1:40:1:46/1"}},
		{"no blank", "if true; then;print x; fi\n", []string{"then 1:10:1:15/1"}},
		{"blank before", "if true; then ; print x; fi\n", []string{"then 1:10:1:17/1"}},
		{"tab before", "if true; then\t;print x; fi\n", []string{"then 1:10:1:16/1"}},
		{"two separators", "if true; then; ; print x; fi\n", []string{"then 1:10:1:18/1"}},
		{"next line", "if true; then\n; print x; fi\n", []string{"then 1:10:2:3/1"}},
		{"separator then next line", "if true; then;\n; print x; fi\n", []string{"then 1:10:2:3/1"}},
		{"after comment", "if true; then # comment\n; print x; fi\n", []string{"then 1:10:2:3/1"}},
		{"before comment", "if true; then; # comment\nprint x; fi\n", []string{"then 1:10:2:1/1"}},
		{"else next line", "if true; then x; else\n; print y; fi\n", []string{"then 1:10:1:15/1", "else 1:18:2:3/1"}},
		{"nested", "if true; then; if true; then; print x; fi; fi\n", []string{"then 1:10:1:16/1", "then 1:25:1:31/1"}},
		{"inside do separator loop", "while true; do; if true; then; print x; fi; break; done\n", []string{"then 1:26:1:32/1"}},
		{"loop inside branch", "if true; then; while true; do; break; done; fi\n", []string{"then 1:10:1:16/1"}},
		{"two ifs", "if true; then; print x; fi; if true; then; print y; fi\n", []string{"then 1:10:1:16/1", "then 1:38:1:44/1"}},
		{"tail after fi", "if true; then; print x; fi && print z\n", []string{"then 1:10:1:16/1"}},
		{"then as data", "for x in then; do print $x; done\nif print then; then; print x; fi\n", []string{"then 2:16:2:22/1"}},
		{"quoted lookalikes", "if true; then; print 'then; x' \"then; y\"; fi\n", []string{"then 1:10:1:16/1"}},
		{"heredoc lookalike", "if true; then; cat <<EOF\nthen;\nEOF\nprint x; fi\n", []string{"then 1:10:1:16/2"}},
		{"empty branches keep parsing", "if true; then; fi\nif true; then; else; fi\n", []string{"then 1:10:-/0", "then 2:10:-/0", "else 2:16:-/0"}},
		{"in command substitution", "if true; then; print $(if true; then; print x; fi); fi\n", []string{"then 1:10:1:16/1", "then 1:33:1:39/1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)
			if got := ifBranches(tree); strings.Join(got, "|") != strings.Join(test.branches, "|") {
				t.Fatalf("branches = %v, want %v", got, test.branches)
			}
			syntax.Walk(tree, func(node syntax.Node) bool {
				clause, ok := node.(*syntax.IfClause)
				if !ok {
					return true
				}
				if clause.ThenPos.IsValid() {
					if got := test.src[clause.ThenPos.Offset() : clause.ThenPos.Offset()+4]; got != "then" {
						t.Errorf("ThenPos %s reads %q in the source, want \"then\"", clause.ThenPos, got)
					}
				} else if got := test.src[clause.Position.Offset() : clause.Position.Offset()+4]; got != "else" {
					t.Errorf("else Position %s reads %q in the source, want \"else\"", clause.Position, got)
				}
				return true
			})
		})
	}
}

// Every failing row is gated by one of the two error texts before the
// adapter runs; a row the base parser accepts tests nothing.
func TestParseThenLeadingSeparatorGatesOnTheParserError(t *testing.T) {
	for _, src := range []string{
		"if true; then; print x; fi\n",
		"if true; then x; else; y; fi\n",
		"if true; then x; elif false; then; y; fi\n",
		"if true; then\n; print x; fi\n",
	} {
		_, firstErr := parseTree([]byte(src), "gate.zsh")
		var parseErr syntax.ParseError
		if !errors.As(firstErr, &parseErr) || !isLeadingSeparatorError(parseErr.Text, ifEmptyBranchErrors) {
			t.Errorf("parseTree(%q) error = %v, want an empty branch error", src, firstErr)
		}
	}
}

// The adapter must not move any other node: the tree of a branch with a
// leading separator matches the tree of the same source with a blank in its
// place, byte for byte.
func TestParseThenLeadingSeparatorMatchesBlank(t *testing.T) {
	tests := []struct {
		name string
		src  string
		twin string
	}{
		{"same line", "if true; then; print a; fi\nprint b\n", "if true; then  print a; fi\nprint b\n"},
		{"else", "if true; then a; else; print b; fi\n", "if true; then a; else  print b; fi\n"},
		{"next line", "if true; then\n  ; print x\nfi\n", "if true; then\n    print x\nfi\n"},
		{"after comment", "if true; then # c\n; print x\nfi\n", "if true; then # c\n  print x\nfi\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.twin) != len(test.src) {
				t.Fatalf("twin length %d, want %d", len(test.twin), len(test.src))
			}
			file, err := Parse(strings.NewReader(test.src), "separator.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			twinFile, err := Parse(strings.NewReader(test.twin), "blank.zsh")
			if err != nil {
				t.Fatalf("Parse(twin) error: %v", err)
			}
			var got, want []string
			collect := func(tree *syntax.File, into *[]string) {
				syntax.Walk(tree, func(node syntax.Node) bool {
					if node == nil {
						return true
					}
					*into = append(*into, fmt.Sprintf("%T %s %s", node, node.Pos(), node.End()))
					return true
				})
			}
			collect(file.AST(), &got)
			collect(twinFile.AST(), &want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("nodes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// Native Zsh rejects each fixture (`zsh -f -n`), so the front end must too,
// with the parser's own error family at the original position.
func TestParseThenLeadingSeparatorRejectsInvalidSources(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-297-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 14},
		{"invalid-297-ampersand-after-separator.txt", "`&` can only immediately follow a statement", 1, 16},
		{"invalid-297-then-outside-if.txt", "`then` can only be used in an `if`", 1, 1},
		{"invalid-297-unterminated-body.txt", "`if` statement must end with `fi`", 1, 1},
		{"invalid-297-else-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 22},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			assertParseErrorAt(t, src, test.text, test.line, test.col)
		})
	}
}

// A `;` that opens a loop body fails the same way in the parser but is a
// different construct; the adapter finds no `then` or `else` site and hands
// the error on without a retry.
func TestParseThenLeadingSeparatorLeavesOtherErrorsUntouched(t *testing.T) {
	for _, src := range []string{"while true; do; break; done\n", "print x }\n"} {
		src := []byte(src)
		_, firstErr := parseTree(src, "other.zsh")
		if firstErr == nil {
			t.Fatalf("parseTree(%q) unexpectedly succeeded", src)
		}
		calls := 0
		_, err := parseThenLeadingSeparatorWithParser(src, "other.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
			calls++
			return nil, nil
		})
		if err != firstErr {
			t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
		}
		if calls != 0 {
			t.Fatalf("parser called %d times, want 0", calls)
		}
	}
}

// A retry whose tree does not hold the branch at the masked site is not
// trusted: the parser error is returned instead.
func TestParseThenLeadingSeparatorFailsClosedWithoutIf(t *testing.T) {
	src := []byte("if true; then; print x; fi\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the leading separator")
	}
	_, err := parseThenLeadingSeparatorWithParser(src, "closed.zsh", firstErr, func(masked []byte, _ string) (*syntax.File, error) {
		if string(masked) != "if true; then  print x; fi\n" {
			t.Fatalf("masked source = %q", masked)
		}
		return &syntax.File{}, nil
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
}
