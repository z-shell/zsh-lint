package parse

import (
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
