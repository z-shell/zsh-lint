package parse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #327: a while or until loop header whose body is the empty sublist
// native Zsh reads before a closer, before a pipeline/list operator, or at EOF.
func TestWhileEmptyBody(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		until     bool
		checkCond func(t *testing.T, clause *syntax.WhileClause)
	}{
		{
			name:  "while true",
			src:   "while true\n",
			until: false,
		},
		{
			name:  "until true",
			src:   "until true\n",
			until: true,
		},
		{
			name:  "while arithmetic",
			src:   "while (( 1 ))\n",
			until: false,
		},
		{
			name:  "until arithmetic",
			src:   "until (( 1 ))\n",
			until: true,
		},
		{
			name:  "while test",
			src:   "while [[ -n $x ]]\n",
			until: false,
		},
		{
			// Native par_while reads undelimited commands as the condition,
			// so `true print x` is one command and the body is the empty sublist at EOF.
			name:  "while undelimited command",
			src:   "while true print x\n",
			until: false,
			checkCond: func(t *testing.T, clause *syntax.WhileClause) {
				if len(clause.Cond) != 1 {
					t.Fatalf("Cond holds %d statements, want 1", len(clause.Cond))
				}
				call, ok := clause.Cond[0].Cmd.(*syntax.CallExpr)
				if !ok {
					t.Fatalf("condition command = %T, want *syntax.CallExpr", clause.Cond[0].Cmd)
				}
				if len(call.Args) != 3 {
					t.Errorf("condition holds %d words, want 3 (`true print x` is one command)", len(call.Args))
				}
			},
		},
		{
			// Trailing semicolon after the condition is skipped before the empty body.
			name:  "while with semicolon",
			src:   "while true;\n",
			until: false,
		},
		{
			name:  "while in function",
			src:   "f() { while true }\n",
			until: false,
		},
		{
			name:  "until in function",
			src:   "g() { until true }\n",
			until: true,
		},
		{
			name:  "while in brace group",
			src:   "{ while true }\n",
			until: false,
		},
		{
			name:  "while in brace group with semicolon",
			src:   "{ while true; }\n",
			until: false,
		},
		{
			name:  "while in subshell",
			src:   "( while true )\n",
			until: false,
		},
		{
			name:  "while in if body",
			src:   "if true; then while true; fi\n",
			until: false,
		},
		{
			// An OR operator taking the loop as its left operand ends the empty body.
			name:  "while with or list",
			src:   "v() { while true || print x }\n",
			until: false,
		},
		{
			// A pipeline operator taking the loop as its left operand ends the empty body.
			name:  "while with pipeline",
			src:   "w() { while true | cat }\n",
			until: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			loops := whileLoops(file.AST())
			if len(loops) != 1 {
				t.Fatalf("while loops = %d, want 1", len(loops))
			}
			loop := loops[0]
			if len(loop.Do) != 0 {
				t.Errorf("len(Do) = %d, want 0", len(loop.Do))
			}
			if loop.Until != test.until {
				t.Errorf("Until = %v, want %v", loop.Until, test.until)
			}
			if loop.DoPos != loop.DonePos {
				t.Errorf("DoPos = %v, DonePos = %v, want DoPos == DonePos", loop.DoPos, loop.DonePos)
			}
			if test.checkCond != nil {
				test.checkCond(t, loop)
			}
		})
	}
}

// The adapter inserts synthetic do and done through a source map and rebases,
// so all nodes in the resulting WhileClause must retain original source offsets,
// and the synthetic semicolon inserted for the parser retry must be cleared.
func TestWhileEmptyBodyPositions(t *testing.T) {
	src := "f() { while (( 1 )) }\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	loops := whileLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("while loops = %d, want 1", len(loops))
	}
	loop := loops[0]

	// WhilePos is the byte offset of the keyword in original source ("while" at offset 6).
	if got := int(loop.WhilePos.Offset()); got != 6 {
		t.Errorf("WhilePos offset = %d, want 6", got)
	}
	if got := src[loop.WhilePos.Offset() : loop.WhilePos.Offset()+5]; got != "while" {
		t.Errorf("WhilePos text = %q, want \"while\"", got)
	}

	// Cond statement positions point at original offsets ("(( 1 ))" at offset 12..21).
	if len(loop.Cond) != 1 {
		t.Fatalf("len(Cond) = %d, want 1", len(loop.Cond))
	}
	cond := loop.Cond[0]
	if got := int(cond.Pos().Offset()); got != 12 {
		t.Errorf("cond Pos offset = %d, want 12", got)
	}
	if got := int(cond.End().Offset()); got != 19 {
		t.Errorf("cond End offset = %d, want 19", got)
	}
	if got := src[cond.Pos().Offset():cond.End().Offset()]; got != "(( 1 ))" {
		t.Errorf("cond text = %q, want \"(( 1 ))\"", got)
	}

	// Synthetic separator on the last Cond statement must be cleared.
	if cond.Semicolon != (syntax.Pos{}) {
		t.Errorf("last Cond Semicolon = %v, want zero Pos", cond.Semicolon)
	}

	// Both keywords of the empty body sit on the closer's own byte, the '}'
	// at offset 20, which is what proves neither points into inserted text.
	if loop.DoPos != loop.DonePos {
		t.Errorf("DoPos = %v, DonePos = %v, want DoPos == DonePos", loop.DoPos, loop.DonePos)
	}
	if got := int(loop.DoPos.Offset()); got != 20 {
		t.Errorf("DoPos offset = %d, want 20", got)
	}
	if got := src[loop.DoPos.Offset()]; got != '}' {
		t.Errorf("DoPos byte = %q, want '}'", got)
	}
}

// Native Zsh rejects each fixture, so the front end must too, returning
// a syntax.ParseError.
func TestWhileEmptyBodyRejectsInvalidSources(t *testing.T) {
	fixtures := []string{
		"invalid-327-stray-brace-after-header.txt",
		"invalid-327-closer-before-header.txt",
		"invalid-327-case-terminator-after-header.txt",
		"invalid-327-until-case-terminator.txt",
		"invalid-327-stray-brace-after-do-form.txt",
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), fixture)
			if err == nil {
				t.Fatalf("Parse(%s) unexpectedly succeeded", fixture)
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("Parse(%s) error type = %T, want syntax.ParseError: %v", fixture, err, err)
			}
		})
	}
}

// The adapter gate only acts on errors it owns; unrelated parser errors
// are returned unchanged without invoking the parser retry.
func TestWhileEmptyBodyDeclinesUnrelatedErrors(t *testing.T) {
	src := []byte("while true\n")
	unrelated := syntax.ParseError{
		Filename: "unrelated.zsh",
		Pos:      syntax.NewPos(0, 1, 1),
		Text:     "`[` must be followed by an expression",
	}
	calls := 0
	_, err := parseWhileEmptyBodyWithParser(src, "unrelated.zsh", unrelated, func([]byte, string) (*syntax.File, error) {
		calls++
		return nil, nil
	})
	if !errors.Is(err, unrelated) {
		t.Fatalf("error = %v, want incoming error %v", err, unrelated)
	}
	if calls != 0 {
		t.Fatalf("parser called %d times, want 0", calls)
	}
}

// Regression guards: standard loop forms and the issue #211 delimited short form
// must remain unaffected by the empty body adapter.
func TestWhileEmptyBodyExistingForms(t *testing.T) {
	t.Run("while with do done", func(t *testing.T) {
		src := "while true; do :; done\n"
		file, err := Parse(strings.NewReader(src), "while_do.zsh")
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		loops := whileLoops(file.AST())
		if len(loops) != 1 {
			t.Fatalf("while loops = %d, want 1", len(loops))
		}
		if len(loops[0].Do) != 1 {
			t.Fatalf("Do has %d statements, want 1", len(loops[0].Do))
		}
	})
	t.Run("while delimited short form", func(t *testing.T) {
		src := "while (( i < 3 )) (( i++ ))\n"
		file, err := Parse(strings.NewReader(src), "while_short.zsh")
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		loops := whileLoops(file.AST())
		if len(loops) != 1 {
			t.Fatalf("while loops = %d, want 1", len(loops))
		}
		if len(loops[0].Do) != 1 {
			t.Fatalf("Do has %d statements, want 1", len(loops[0].Do))
		}
	})
	t.Run("for loop", func(t *testing.T) {
		src := "for i in a b; do :; done\n"
		if _, err := Parse(strings.NewReader(src), "for.zsh"); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
	})
}

// scanWhileDoForms identifies loops whose body is already a do...done list
// so their keywords are not blanked in the probe.
func TestWhileEmptyBodyScanWhileDoForms(t *testing.T) {
	t.Run("reports keyword for do form", func(t *testing.T) {
		src := []byte("while true; do :; done\n")
		forms := scanWhileDoForms(src)
		if !forms[0] {
			t.Fatalf("scanWhileDoForms() did not report keyword offset 0, got %v", forms)
		}
	})
	t.Run("does not report keyword for empty body form", func(t *testing.T) {
		src := []byte("while true\n")
		forms := scanWhileDoForms(src)
		if forms[0] {
			t.Fatalf("scanWhileDoForms() unexpectedly reported keyword offset 0, got %v", forms)
		}
	})
}
