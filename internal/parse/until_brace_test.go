package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// `until list { list }` is the alternate form next to `while list { list }`
// (#394): the brace after the condition list is the body. Each row passes
// `zsh -f -n`.
func TestParseUntilBraceBody(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		until   int // offset of `until`
		brace   int // offset of the body's `{`
		condLen int
		bodyLen int
	}{
		{"test", "until [[ a ]] { print B; break }\n", 0, 14, 1, 2},
		{"arithmetic", "until (( 1 )) { : }\n", 0, 14, 1, 1},
		{"test list", "until [[ a ]] && [[ b ]] { : }\n", 0, 25, 1, 1},
		{"plain command heads the list", "until true && [[ a ]] { : }\n", 0, 22, 1, 1},
		{"multiline body", "until (( 1 )) {\n:\n}\n", 0, 14, 1, 1},
		{"empty body", "until (( 1 )) { }\n", 0, 14, 1, 0},
		{"in a function", "f() {\nuntil (( 1 )) { : }\n}\n", 6, 20, 1, 1},
		{"in a classic if", "if true; then\nuntil (( 1 )) { : }\nfi\n", 14, 28, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "until.zsh")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			var loops []*syntax.WhileClause
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if loop, ok := node.(*syntax.WhileClause); ok {
					loops = append(loops, loop)
				}
				return true
			})
			if len(loops) != 1 {
				t.Fatalf("found %d loops, want 1", len(loops))
			}
			loop := loops[0]
			if !loop.Until {
				t.Error("Until = false, want true")
			}
			if got := int(loop.WhilePos.Offset()); got != tc.until {
				t.Errorf("WhilePos offset = %d, want %d", got, tc.until)
			}
			if got := int(loop.DoPos.Offset()); got != tc.brace {
				t.Errorf("DoPos offset = %d, want %d", got, tc.brace)
			}
			if len(loop.Cond) != tc.condLen {
				t.Errorf("len(Cond) = %d, want %d", len(loop.Cond), tc.condLen)
			}
			if len(loop.Do) != tc.bodyLen {
				t.Fatalf("len(Do) = %d, want %d", len(loop.Do), tc.bodyLen)
			}
			if tc.bodyLen > 0 {
				if got := int(loop.Do[0].Pos().Offset()); got <= tc.brace {
					t.Errorf("body starts at %d, want after the brace at %d", got, tc.brace)
				}
			}
		})
	}
}

// A brace on the line after the condition is one more condition element, for
// `until` as for `while` (#330): `until (( i >= 1 ))`, newline,
// `{ print X; false }` runs the group once and never a body.
func TestParseUntilBraceOnNextLineStaysCondition(t *testing.T) {
	for _, src := range []string{
		"until (( 1 ))\n{ : }\n",
		"until [[ a ]]\n{ : }\n",
		"until [[ a ]] # c\n{ : }\n",
		"until (( 0 ))\n{ print X; false }\nprint end\n",
	} {
		file, err := Parse(strings.NewReader(src), "until.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) failed: %v", src, err)
		}
		loop, ok := file.AST().Stmts[0].Cmd.(*syntax.WhileClause)
		if !ok {
			t.Fatalf("%q: Cmd is %T, want a loop", src, file.AST().Stmts[0].Cmd)
		}
		if !loop.Until || len(loop.Do) != 0 || len(loop.Cond) < 2 {
			t.Errorf("%q: Until=%v len(Cond)=%d len(Do)=%d, want an until loop whose condition holds the brace",
				src, loop.Until, len(loop.Cond), len(loop.Do))
		}
	}
}

// Rows rejected by `zsh -f -n` stay rejected: a plain command must not be the
// last condition element before the brace, and the braces must balance.
func TestParseUntilBraceBodyKeepsRejecting(t *testing.T) {
	for _, src := range []string{
		"until true { : }\n",
		"until ! false { : }\n",
		"until true && true { : }\n",
		"until false || true { : }\n",
		"until print | read { : }\n",
		"until (( 1 )) {\n",
		"until (( 1 )) { : } }\n",
	} {
		if _, err := Parse(strings.NewReader(src), "until.zsh"); err == nil {
			t.Errorf("invalid Zsh must stay rejected: %q", src)
		}
	}
}

// Adding `until` exposed two scanner slips that `while` already had; each row
// passes `zsh -f -n`, and the tree is the one Zsh runs.
func TestParseAlternateLoopScannerResumesAfterArithmetic(t *testing.T) {
	type loopShape struct {
		until            bool
		condLen, bodyLen int
	}
	for _, tc := range []struct {
		name  string
		src   string
		loops []loopShape
		ifs   int
	}{
		// A comment after `(( ))` holds a `{` that is not the body: the
		// scanner stepped over the blank before `#` as a word byte, so the
		// comment's braces were counted.
		{"comment after an arithmetic condition", "if [[ a ]] {\nwhile (( 1 )) # c { : } }\n}\n", []loopShape{{false, 1, 0}}, 1},
		{"comment after an arithmetic until", "if [[ a ]] {\nuntil (( 1 )) # c { : } }\n}\n", []loopShape{{true, 1, 0}}, 1},
		// The byte after `))` was consumed the same way, so a newline or `;`
		// there no longer started a command and the brace-form `if` after
		// it went unseen. Zsh runs it as a second condition element.
		{"brace-form if on the next line", "while (( 0 ))\nif [[ a ]] { : }\ndo :; done\n", []loopShape{{false, 2, 1}}, 1},
		{"brace-form if after a semicolon", "while (( 0 ));if [[ a ]] { : }\ndo :; done\n", []loopShape{{false, 2, 1}}, 1},
		// A condition group's body brace sits on its line; after a newline
		// the brace is one more condition element (#330), for `while` as
		// for `until`. Inside a brace-form body the old newline search
		// claimed that brace and broke the enclosing block.
		{"group condition, brace on the next line", "if [[ a ]] {\nwhile { true }\n{ : }\n}\n", []loopShape{{false, 2, 0}}, 1},
		{"until group condition, brace on the next line", "if [[ a ]] {\nuntil { true }\n{ : }\n}\n", []loopShape{{true, 2, 0}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), "loop.zsh")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			var loops []loopShape
			ifs := 0
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				switch n := node.(type) {
				case *syntax.WhileClause:
					loops = append(loops, loopShape{n.Until, len(n.Cond), len(n.Do)})
				case *syntax.IfClause:
					ifs++
				}
				return true
			})
			if len(loops) != len(tc.loops) || ifs != tc.ifs {
				t.Fatalf("found %d loops and %d ifs, want %d and %d", len(loops), ifs, len(tc.loops), tc.ifs)
			}
			for k, want := range tc.loops {
				if loops[k] != want {
					t.Errorf("loop %d = %+v, want %+v", k, loops[k], want)
				}
			}
		})
	}
}
