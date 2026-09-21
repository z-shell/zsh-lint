package parse

import (
	"errors"
	"os"
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
			name:      "length expansion in body",
			src:       "if (( x )) { print ${#a} }\n",
			wantStmts: 1,
		},
		{
			name:      "length expansion in else body",
			src:       "if (( x )) { print x; } else { print ${#a} }\n",
			wantStmts: 1,
		},
		{
			name:      "nested expansion with hash in body",
			src:       "if (( x )) { print ${${a[1]}#b} ${a:#b} $#a \"${#a}\" }\n",
			wantStmts: 1,
		},
		{
			name:      "hash after a space inside an expansion in body",
			src:       "if (( x )) { a=${b## ##} } else { a=${${c## ##}%% %%} }\n",
			wantStmts: 1,
		},
		{
			name:      "braces quotes and keywords inside expansions in body",
			src:       "if (( x )) { a=${b:-\"}\"} c=${d:-'}'} e=${f//\\}/x} g=${h:-{1,2}} k=${l:-if} m=${n:-a;b} } elif (( y )) { print ${o:-a|b} } else { print ${p:-a&b} }\n",
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

// Issue #230: `${` inside a brace-form if body was scanned as a block opener,
// so the `#` of `${#a}` started a comment that swallowed the closing brace.
// The retry is byte-length preserving, so the body keeps its position and the
// expansion its original text.
func TestParseAlternateIfLengthExpansionInBody(t *testing.T) {
	src := "if (( x )) { print ${#a} } else { print ${#b} }\n"
	file, err := Parse(strings.NewReader(src), "length.zsh")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ifc, ok := file.AST().Stmts[0].Cmd.(*syntax.IfClause)
	if !ok {
		t.Fatalf("Cmd is not *syntax.IfClause: %T", file.AST().Stmts[0].Cmd)
	}
	if len(ifc.Then) != 1 || ifc.Else == nil || len(ifc.Else.Then) != 1 {
		t.Fatalf("unexpected clause shape: then=%d else=%v", len(ifc.Then), ifc.Else != nil)
	}
	call, ok := ifc.Then[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 2 {
		t.Fatalf("Then[0] is not a two-word call: %T", ifc.Then[0].Cmd)
	}
	if got := call.Args[0].Pos(); got.Line() != 1 || got.Col() != 14 {
		t.Errorf("Then[0] print position = %v, want 1:14", got)
	}
	pe, ok := call.Args[1].Parts[0].(*syntax.ParamExp)
	if !ok || !pe.Length || pe.Param.Value != "a" {
		t.Fatalf("Then[0] argument is not ${#a}: %#v", call.Args[1].Parts[0])
	}
	if got := pe.Pos(); got.Line() != 1 || got.Col() != 20 {
		t.Errorf("${#a} position = %v, want 1:20", got)
	}
}

// A `\`-newline before else or elif is removed by the lexer, so the chain is
// the same as on one line. Issue #207: the scanner used to end the if at the
// `}` and orphan the else. Both sources are `zsh -f -n` valid.
func TestParseAlternateIfChainAcrossLineContinuation(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantElse bool
	}{
		{
			name:     "else after continuation",
			src:      "if (( x == y )) { success } \\\nelse { failure }\n",
			wantElse: true,
		},
		{
			name:     "elif after continuation",
			src:      "if (( x )) { a } \\\nelif (( y )) { b }\n",
			wantElse: true,
		},
		{
			name:     "continuation before every brace",
			src:      "if [[ -n $y ]] \\\n{ c } \\\nelif (( z )) \\\n{ d } \\\nelse \\\n{ e }\n",
			wantElse: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			if len(file.AST().Stmts) != 1 {
				t.Fatalf("len(Stmts) = %d, want 1", len(file.AST().Stmts))
			}
			ifc, ok := file.AST().Stmts[0].Cmd.(*syntax.IfClause)
			if !ok {
				t.Fatalf("Cmd is not *syntax.IfClause: %T", file.AST().Stmts[0].Cmd)
			}
			if (ifc.Else != nil) != test.wantElse {
				t.Errorf("Else present = %v, want %v", ifc.Else != nil, test.wantElse)
			}
			if ifc.Else != nil && ifc.Else.Pos().Line() < 2 {
				t.Errorf("Else.Pos() = %v, want the continuation line", ifc.Else.Pos())
			}
		})
	}
}

// When the retry on the transformed source fails, the error must point into
// the original file. The synthetic `; then` and `fi` lines used to shift it
// (issue #207: a 2-line file reported line 5).
func TestParseAlternateIfRetryErrorKeepsOriginalPosition(t *testing.T) {
	src := "if (( x )) { a }\nprint one\nfi\n"
	_, err := Parse(strings.NewReader(src), "retry.zsh")
	var perr syntax.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
	}
	if perr.Pos.Line() != 3 || perr.Pos.Col() != 1 {
		t.Errorf("position = %d:%d, want 3:1 (the stray fi)", perr.Pos.Line(), perr.Pos.Col())
	}
}

// A newline, `;`, or comment after the closing brace ends a brace-form if;
// an else or elif on the next line belongs to the enclosing classic if.
// Native Zsh rejects `if [[ x ]] { : }` newline `else { : }`, so the chain
// cannot continue there. Minimized from z-shell/zi zi.zsh:523-558.
func TestParseAlternateIfEndsAtNewlineBeforeElse(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"classic else after nested brace if", "if [[ $x = 2 ]]; then\n  if (( y )) { : }\nelse\n  :\nfi\n"},
		{"classic elif after nested brace chain", "if [[ a ]]; then\n  if (( y )) { : } else { : }\nelif [[ b ]]; then\n  :\nfi\n"},
		{"loop body", "for f; do\n  if [[ a ]]; then\n    if (( y )) { : } else { : }\n  else\n    :\n  fi\ndone\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			if len(file.AST().Stmts) != 1 {
				t.Fatalf("len(Stmts) = %d, want 1", len(file.AST().Stmts))
			}
		})
	}
	for _, src := range []string{
		"if [[ x ]] { : }\nelse { : }\n",
		"if [[ x ]] { : } ; else { : }\n",
		"if [[ x ]] { : } # comment\nelse { : }\n",
	} {
		if _, err := Parse(strings.NewReader(src), "invalid.zsh"); err == nil {
			t.Errorf("Parse(%q) error = nil, want a parse error (native Zsh rejects it)", src)
		}
	}
}

// Issue #220: native Zsh rejects `if [[ x ]]` newline `{ : }`, while
// accepting `while (( x ))` newline `{ : }` and `\`-newline continuations.
func TestParseAlternateIfRejectsNewlineBeforeBrace(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-220-newline-if-brace.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	if _, err := Parse(strings.NewReader(string(fixture)), "invalid-220.zsh"); err == nil {
		t.Fatal("Parse() unexpectedly accepted invalid-220 fixture with newline before brace")
	}

	invalidSources := []struct {
		name string
		src  string
	}{
		{"double bracket newline", "if [[ x ]]\n{ : }\n"},
		{"arithmetic newline", "if (( x ))\n{ : }\n"},
		{"elif double bracket newline", "if [[ x ]] { : } elif [[ y ]]\n{ : }\n"},
		{"brace condition newline", "if { true }\n{ : }\n"},
	}
	for _, tc := range invalidSources {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tc.src), tc.name+".zsh"); err == nil {
				t.Errorf("Parse(%q) error = nil, want a parse error (native Zsh rejects it)", tc.src)
			}
		})
	}
}

func TestParseAlternateIfAllowsWhileAndContinuationNewlines(t *testing.T) {
	validSources := []struct {
		name string
		src  string
	}{
		{"while arithmetic condition", "while (( count < 3 )) {\n  (( count++ ))\n}\n"},
		{"if double bracket continuation", "if [[ x ]] \\\n{ : }\n"},
		{"if arithmetic continuation", "if (( x )) \\\n{ : }\n"},
		{"else newline", "if [[ x ]] { : } else\n{ : }\n"},
	}
	for _, tc := range validSources {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), tc.name+".zsh")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			if len(file.AST().Stmts) != 1 {
				t.Fatalf("len(Stmts) = %d, want 1", len(file.AST().Stmts))
			}
		})
	}
}

// Issue #254: the condition of a brace-form if, elif, or while ends only at a
// `]]` that is a whole word. A `]]` inside a bracket class, a pattern, or a
// quoted string belongs to the condition, exactly as the parser reads it in
// the classic `; then` form. Minimized from z-shell/F-Sy-H lib/highlight.zsh:747.
func TestParseAlternateIfConditionKeepsEmbeddedDoubleBrackets(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKind string
	}{
		{"bracket class in group", "if [[ $a == ([\\]]) ]] { b=1 }\n", "if"},
		{"negated bracket class", "if [[ $a == [^\\]] ]] { b=1 }\n", "if"},
		{"leading close in class", "if [[ $a == ([]]) ]] { b=1 }\n", "if"},
		{"class followed by close", "if [[ $a == [\\]]] ]] { b=1 }\n", "if"},
		{"character class name", "if [[ $a == [[:alpha:]] ]] { b=1 }\n", "if"},
		{"glued to a word", "if [[ $a == x]] ]] { b=1 }\n", "if"},
		{"double quoted", "if [[ $a == \"]]\" ]] { b=1 }\n", "if"},
		{"single quoted", "if [[ $a == ']]' ]] { b=1 }\n", "if"},
		{"spaced inside quotes", "if [[ $a == \"x ]] y\" ]] { b=1 }\n", "if"},
		{"subscript close", "if [[ $a == $b[1]] ]] { b=1 }\n", "if"},
		{"while", "while [[ $a == ([\\]]) ]] { b=1 }\n", "while"},
		{"elif", "if [[ $a == x ]] { b=1 } elif [[ $a == [^\\]] ]] { b=2 }\n", "if"},
		{"chained condition", "if [[ $a == x ]] && [[ $b == \"]]\" ]] { b=1 }\n", "if"},
		{"group close glued to the terminator", "if [[ ( $a == x )]] { b=1 }\n", "if"},
		{"unspaced group glued to the terminator", "if [[ ($a == x)]] { b=1 }\n", "if"},
		{"line continuation before the terminator", "if [[ $a == x \\\n]] { b=1 }\n", "if"},
		{"ansi-c quoted with an escaped quote", "if [[ $a == $'x\\' ]] y' ]] { b=1 }\n", "if"},
		{"ansi-c quoted", "if [[ $a == $'x ]] y' ]] { b=1 }\n", "if"},
		{"pattern group glued to a class close", "if [[ $a == (x)]] ]] { b=1 }\n", "if"},
		{"escaped paren before a class close", "if [[ $a == [\\)]] ]] { b=1 }\n", "if"},
		{"joined groups glued to the terminator", "if [[ ($a == x)&&($b == y)]] { b=1 }\n", "if"},
		{"negated group glued to the terminator", "if [[ ! ($a == x)]] { b=1 }\n", "if"},
		{"joiner inside a word", "if [[ $a == x&&$b == y ]] { b=1 }\n", "if"},
		{"spaced command substitution", "if [[ $( f ) == x ]] { b=1 }\n", "if"},
		{"spaced arithmetic expansion", "if [[ $(( a + 1 )) -gt 2 ]] { b=1 }\n", "if"},
		{"spaced pattern group", "if [[ $a == (x|y z) ]] { b=1 }\n", "if"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", test.src, err)
			}
			stmts := file.AST().Stmts
			if len(stmts) != 1 {
				t.Fatalf("len(Stmts) = %d, want 1", len(stmts))
			}
			var body []*syntax.Stmt
			switch cmd := stmts[0].Cmd.(type) {
			case *syntax.IfClause:
				if test.wantKind != "if" {
					t.Fatalf("Cmd is %T, want %s", cmd, test.wantKind)
				}
				body = cmd.Then
			case *syntax.WhileClause:
				if test.wantKind != "while" {
					t.Fatalf("Cmd is %T, want %s", cmd, test.wantKind)
				}
				body = cmd.Do
			default:
				t.Fatalf("Cmd is %T, want an if or while clause", cmd)
			}
			if len(body) != 1 {
				t.Fatalf("len(body) = %d, want 1", len(body))
			}
			// The body is the first `b=` in the source; the retry must keep it there.
			bodyOffset := strings.Index(test.src, "b=")
			wantLine := uint(strings.Count(test.src[:bodyOffset], "\n") + 1)
			wantCol := uint(bodyOffset - strings.LastIndex(test.src[:bodyOffset], "\n"))
			if pos := body[0].Pos(); pos.Line() != wantLine || pos.Col() != wantCol {
				t.Errorf("body position = %d:%d, want %d:%d", pos.Line(), pos.Col(), wantLine, wantCol)
			}
		})
	}
}

// A `]]` glued to the following `{` is not the closing word, in Zsh or in the
// parser, so the brace-form retry must leave the parser error alone.
func TestParseAlternateIfRejectsBraceGluedToDoubleBracket(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-254-brace-glued-to-double-bracket.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	assertParseErrorAt(t, fixture, "not a valid test operator: `]]{`", 1, 15)
	assertParseErrorAt(t, []byte("while [[ $a == x ]]{ b=1 }\n"), "not a valid test operator: `]]{`", 1, 18)
	// A word glued to `]]` is not the closing word either, in Zsh or in the parser.
	assertParseErrorAt(t, []byte("if [[ -n $a]] { b=1 }\n"), "not a valid test operator: `{`", 1, 15)
	assertParseErrorAt(t, []byte("if [[ $a == \"x\"]] { b=1 }\n"), "not a valid test operator: `{`", 1, 19)
	// An unescaped `)` inside a bracket class is a parse error in Zsh too, so
	// the adapter leaves the parser's own error in place.
	assertParseErrorAt(t, []byte("if [[ $a == [)]] ]] { b=1 }\n"), "reached `)` without matching `[[` with `]]`", 1, 4)
}

// A `while` whose condition is not followed by a brace has an ordinary body,
// so the alternate-form scan must disarm at that point. It searches past
// newlines for the `{`, so staying armed let a brace on a later line be taken
// as the loop body: the `{ ... }` of a following `try`/`always` block was
// masked into a `do ... done`, and valid Zsh was rejected at the `always`
// (#337). Each statement here parses alone and in pairs; only all three
// together reproduced it.
func TestParseAlternateIfDisarmsWhileWithoutBraceBody(t *testing.T) {
	sources := []string{
		"#!/usr/bin/env zsh\nif (( 1 )) { x=1 }\nwhile (( i < 3 )) (( i++ ))\n{ true } always { true }\n",
		"#!/usr/bin/env zsh\nif (( 1 )) { x=1 }\nuntil (( d )) (( d=1 ))\n{ : } always { : }\n",
		"#!/usr/bin/env zsh\nif [[ -n $HOME ]] { y=2 }\nwhile (( a )) print loop\n{ print body } always { print cleanup }\n",
	}
	for _, src := range sources {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse:\n%s\nerror: %v", src, err)
		}
	}

	// The alternate form itself must keep working: a brace directly after the
	// condition is still a loop body, and disarming must not reach it.
	file, err := Parse(strings.NewReader("while (( i < 3 )) { print $i }\n"), "t.zsh")
	if err != nil {
		t.Fatalf("alternate while form must still parse: %v", err)
	}
	loops := whileLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("want 1 while loop, got %d", len(loops))
	}
	if got := len(loops[0].Do); got != 1 {
		t.Errorf("want 1 statement in the loop body, got %d", got)
	}
}
