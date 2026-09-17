package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseMultiNameFor(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantErr   bool
		wantStmts int
	}{
		{
			name: "multi-name for loop with in",
			src: `for key value in a 1 b 2; do
  print -r -- "$key=$value"
done
`,
			wantStmts: 1,
		},
		{
			name: "three names in for loop",
			src: `for a b c in 1 2 3 4 5 6; do
  print -r -- "$a $b $c"
done
`,
			wantStmts: 1,
		},
		{
			name: "short for loop with parens and braces",
			src: `for item ( one two three ) {
  print -r -- "$item"
}
`,
			wantStmts: 1,
		},
		{
			name: "short for loop with parameter expansion",
			src: `for sni ( ${snippets[@]} ) {
  if [[ -n $sni ]]; then
    print $sni
  fi
}
`,
			wantStmts: 1,
		},
		{
			name: "short for loop with list wrapped after the opening paren",
			src: `for item (
  one two
  three
) {
  print -r -- "$item"
}
`,
			wantStmts: 1,
		},
		{
			name: "short for loop with list wrapped before the closing paren",
			src: `for item ( one two
  three ) {
  print -r -- "$item"
}
`,
			wantStmts: 1,
		},
		{
			name: "short for loop with blank lines inside the list",
			src: `for item (
  one

  two
) {
  print -r -- "$item"
}
`,
			wantStmts: 1,
		},
		{
			name:    "invalid for loop syntax",
			src:     `for k v ;`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tt.src), tt.name)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, wanted error", tt.src)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tt.src, err)
			}
			if len(file.AST().Stmts) != tt.wantStmts {
				t.Errorf("len(Stmts) = %d, want %d", len(file.AST().Stmts), tt.wantStmts)
			}
		})
	}
}

// TestParseMultiNameForRebasesRetryErrorPosition regression-tests issue #255:
// a failed retry on the alternate `for name ( words ) { ... }` sugar must
// rebase its error position into the original source, not the longer
// transformed text the retry actually parsed.
func TestParseMultiNameForRebasesRetryErrorPosition(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-255-multi-name-for-retry-later-blocker.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	assertParseErrorAt(t, fixture, "statements must be separated by &, ; or a newline", 2, 12)
}

func TestParseMultiNameForPositions(t *testing.T) {
	src := `for key value in a 1 b 2; do
  print -r -- "$key=$value"
done
`
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	stmt := file.AST().Stmts[0]
	forClause, ok := stmt.Cmd.(*syntax.ForClause)
	if !ok {
		t.Fatalf("Cmd is not *syntax.ForClause: %T", stmt.Cmd)
	}

	if forClause.Pos().Line() != 1 || forClause.Pos().Col() != 1 {
		t.Errorf("ForClause.Pos() = %v, want 1:1", forClause.Pos())
	}

	if len(forClause.Do) != 1 {
		t.Fatalf("len(forClause.Do) = %d, want 1", len(forClause.Do))
	}
	doStmt := forClause.Do[0]
	if doStmt.Pos().Line() != 2 || doStmt.Pos().Col() != 3 {
		t.Errorf("doStmt.Pos() = %v, want 2:3", doStmt.Pos())
	}
}

// TestParseMultiNameForMultilineList regression-tests issue #263: a newline
// that separates words inside the parenthesized list of an alternate-form
// for loop is a plain word separator in zsh, but the rewritten
// `for name in words; do` form cannot hold a newline before `do`. Each
// newline that native zsh treats as a separator is masked; a newline that is
// part of a word (escaped, quoted, or inside a nested substitution) is not.
func TestParseMultiNameForMultilineList(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantItems []string
	}{
		{
			name: "newlines at depth zero separate words",
			src: `for item (
  one two
  three
) {
  print -r -- "$item"
}
`,
			wantItems: []string{"one", "two", "three"},
		},
		{
			name: "backslash continuation stays one list",
			src: `for item ( one \
  two ) {
  print -r -- "$item"
}
`,
			wantItems: []string{"one", "two"},
		},
		{
			name: "quoted newline stays inside its word",
			src: `for item ( "one
two" three ) {
  print -r -- "$item"
}
`,
			wantItems: []string{"\"one\ntwo\"", "three"},
		},
		{
			name: "quoted closing paren does not end the list",
			src: `for item ( "one)
two" three
) {
  print -r -- "$item"
}
`,
			wantItems: []string{"\"one)\ntwo\"", "three"},
		},
		{
			name: "escaped closing paren does not end the list",
			src: `for item ( one\) two
  three ) {
  print -r -- "$item"
}
`,
			wantItems: []string{"one\\)", "two", "three"},
		},
		{
			name: "newline inside a command substitution is not a separator",
			src: `for item ($(print one
print two) three) {
  print -r -- "$item"
}
`,
			wantItems: []string{"$(print one\nprint two)", "three"},
		},
		{
			name: "case pattern paren inside a command substitution does not end the list",
			src: `for item (
  $(case y in y) print one;; esac)
  two
) {
  print -r -- "$item"
}
`,
			wantItems: []string{"$(case y in y) print one;; esac)", "two"},
		},
		{
			name: "case pattern paren inside a one-line command substitution does not end the list",
			src: `for item ( $(case y in y) print one;; esac) two ) {
  print -r -- "$item"
}
`,
			wantItems: []string{"$(case y in y) print one;; esac)", "two"},
		},
		{
			name: "paren inside a parameter expansion operator does not end the list",
			src: `for item (
  ${v%)}
  two
) {
  print -r -- "$item"
}
`,
			wantItems: []string{"${v%)}", "two"},
		},
		{
			name: "comment inside a command substitution does not end the list",
			src: `for item (
  $(print one # )
print two)
  three
) {
  print -r -- "$item"
}
`,
			wantItems: []string{"$(print one # )\nprint two)", "three"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tt.src), "multiline-list.zsh")
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			forClause, ok := file.AST().Stmts[0].Cmd.(*syntax.ForClause)
			if !ok {
				t.Fatalf("Cmd is not *syntax.ForClause: %T", file.AST().Stmts[0].Cmd)
			}
			iter, ok := forClause.Loop.(*syntax.WordIter)
			if !ok {
				t.Fatalf("Loop is not *syntax.WordIter: %T", forClause.Loop)
			}
			var items []string
			for _, item := range iter.Items {
				items = append(items, tt.src[item.Pos().Offset():item.End().Offset()])
			}
			if strings.Join(items, "|") != strings.Join(tt.wantItems, "|") {
				t.Errorf("items = %q, want %q", items, tt.wantItems)
			}
		})
	}
}

// TestParseMultiNameForMultilineListPositions asserts that every position
// after a masked list newline still maps to its original line: the masked
// buffer has fewer lines than the source.
func TestParseMultiNameForMultilineListPositions(t *testing.T) {
	src := `for item (
  one two
  three
) {
  print -r -- "$item"
}
`
	file, err := Parse(strings.NewReader(src), "multiline-list-positions.zsh")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	forClause := file.AST().Stmts[0].Cmd.(*syntax.ForClause)
	iter := forClause.Loop.(*syntax.WordIter)
	if len(iter.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(iter.Items))
	}
	wantItemPos := [][2]uint{{2, 3}, {2, 7}, {3, 3}}
	for i, item := range iter.Items {
		if item.Pos().Line() != wantItemPos[i][0] || item.Pos().Col() != wantItemPos[i][1] {
			t.Errorf("Items[%d].Pos() = %v, want %d:%d", i, item.Pos(), wantItemPos[i][0], wantItemPos[i][1])
		}
	}
	if len(forClause.Do) != 1 {
		t.Fatalf("len(forClause.Do) = %d, want 1", len(forClause.Do))
	}
	if doStmt := forClause.Do[0]; doStmt.Pos().Line() != 5 || doStmt.Pos().Col() != 3 {
		t.Errorf("doStmt.Pos() = %v, want 5:3", doStmt.Pos())
	}
	if forClause.DonePos.Line() != 6 || forClause.DonePos.Col() != 1 {
		t.Errorf("forClause.DonePos = %v, want 6:1", forClause.DonePos)
	}
}

// TestParseMultiNameForMultilineListRetryError asserts that a retry failing
// past a wrapped list rebases its error across the masked newlines.
func TestParseMultiNameForMultilineListRetryError(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-263-multi-line-list-later-blocker.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	assertParseErrorAt(t, fixture, "statements must be separated by &, ; or a newline", 7, 12)
}

// TestParseMultiNameForRejectsCommentInsideList documents the conservative
// bail: a `#` comment inside the list is valid zsh (the source is inline
// rather than an `invalid-` fixture for that reason), but masking it would
// drop a comment node the suppression pass may read, so the original parser
// error stands for that loop.
func TestParseMultiNameForRejectsCommentInsideList(t *testing.T) {
	src := []byte(`for p (
  a # comment )
  b
) {
  x=1
}
`)
	assertParseErrorAt(t, src, "`for foo` must be followed by `in`, `do`, `;`, or a newline", 1, 1)
}

// TestParseMultiNameForDeclinesUnlexableList asserts that a list the
// front-end's word lexer cannot read, or that never closes, leaves the loop
// untouched so the raw parser error stands instead of a rewrite built on a
// guessed boundary.
func TestParseMultiNameForDeclinesUnlexableList(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			// mvdan reads a zsh flag group as a literal ending at its first
			// `)`, so the `.` delimiter that follows is not a valid operator.
			name: "flag group delimiter",
			src: `for item ( ${(s.).)v} two ) {
  print -r -- "$item"
}
`,
		},
		{
			name: "list never closes",
			src: `for item ( one two
`,
		},
		{
			name: "operator inside the list",
			src: `for item ( one < two ) {
  print -r -- "$item"
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParseErrorAt(t, []byte(tt.src), "`for foo` must be followed by `in`, `do`, `;`, or a newline", 1, 1)
		})
	}
}

// TestParseMultiNameForKeepsInFormNewlineInvalid asserts that the newline
// mask applies only to the parenthesized list: the `for name in words` form
// with a bare newline before the words is invalid in zsh and stays rejected.
func TestParseMultiNameForKeepsInFormNewlineInvalid(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-263-for-in-bare-newline.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	assertParseErrorAt(t, fixture, "`for foo [in words]` must be followed by `do`", 1, 1)
}
