package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseAlternateIfBrace(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantErr   bool
		wantStmts int
	}{
		{
			name: "double bracket condition",
			src: `if [[ 1 ]] {
  print "yes"
}
`,
			wantStmts: 1,
		},
		{
			name: "arithmetic condition",
			src: `if (( 1 )) {
  print "math"
}
`,
			wantStmts: 1,
		},
		{
			name: "alternate while with arithmetic condition",
			src: `while (( count < 3 )) {
  (( count++ ))
}
`,
			wantStmts: 1,
		},
		{
			name: "brace condition",
			src: `if { true } {
  print "brace"
}
`,
			wantStmts: 1,
		},
		{
			name: "if-elif-else chain",
			src: `if [[ 1 ]] {
  print "one"
} elif [[ 2 ]] {
  print "two"
} else {
  print "three"
}
`,
			wantStmts: 1,
		},
		{
			name: "alternate if with compound conditions",
			src: `if (( first )) && \
  [[ -n $second ]] || [[ $third == yes ]] {
  print -r -- matched
}
`,
			wantStmts: 1,
		},
		{
			name: "nested alternate if",
			src: `if [[ 1 ]] {
  if (( 2 )) {
    print "nested"
  }
}
`,
			wantStmts: 1,
		},
		{
			name: "single line alternate if",
			src: `if [[ 1 ]] { print "single"; }
`,
			wantStmts: 1,
		},
		{
			name: "alternate if with comments and empty body",
			src: `# leading comment
if [[ $PMSPEC != *f* ]] {
  # inner comment
  fpath+=( "${0:h}/functions" )
}
`,
			wantStmts: 1,
		},
		{
			name:    "invalid undelimited if must fail",
			src:     `if true { print "bad"; }`,
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

func TestParseAlternateIfPositions(t *testing.T) {
	src := `if [[ 1 ]] {
  print -r -- "hello"
}
`
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	stmt := file.AST().Stmts[0]
	ifc, ok := stmt.Cmd.(*syntax.IfClause)
	if !ok {
		t.Fatalf("Cmd is not *syntax.IfClause: %T", stmt.Cmd)
	}

	if ifc.Pos().Line() != 1 || ifc.Pos().Col() != 1 {
		t.Errorf("IfClause.Pos() = %v, want 1:1", ifc.Pos())
	}

	if len(ifc.Then) != 1 {
		t.Fatalf("len(ifc.Then) = %d, want 1", len(ifc.Then))
	}
	thenStmt := ifc.Then[0]
	// "  print -r -- "hello"" is on line 2, starting at column 3
	if thenStmt.Pos().Line() != 2 || thenStmt.Pos().Col() != 3 {
		t.Errorf("thenStmt.Pos() = %v, want 2:3", thenStmt.Pos())
	}
}
