package parse

import (
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A `while` or `until` header's condition is a *list*, so it absorbs every
// sublist up to the end of the enclosing list and the body stays empty
// (issue #330). Every source here was checked with `zsh -f -n` as a file.
func TestParseWhileConditionListAcrossNewline(t *testing.T) {
	sources := []string{
		"while true\nprint x",
		"until true\nprint x",
		"while true\nprint a\nprint b",
		"while (( 1 ))\nprint x",
		"h() { while (( 1 )); print x }",
		"h() {\nwhile (( 1 ))\nprint x\n}",
		"( while true\nprint x\n)",
		"while true && false\nprint x",
		"for i in 1 2; do while (( 1 )); print a; false; done",
		// Two bare loops in a row: the second is part of the first's
		// condition list. This is the shape PR #340 (#332) recorded as a
		// remaining limitation.
		"while true\nprint a\nwhile true\nprint b",
		"while true; ;\nprint x",
	}

	for _, src := range sources {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// Widening the condition to the end of its list must not swallow a real
// syntax error that happens to sit after the header. Each source here is
// rejected by `zsh -f -n` as a file and must stay rejected.
func TestParseWhileConditionListKeepsRejecting(t *testing.T) {
	sources := []string{
		"while true\nprint x\n{",
		"while true\nprint x\nfi",
		"while true\nprint x\ndone",
		"while true\nprint x\nif",
		"while true\nprint 'x",
		"while true\nprint x\n)",
		"while true\nprint x\nesac",
		"while true\nprint x\ndo",
	}

	for _, src := range sources {
		if err := parseString(t, src); err == nil {
			t.Errorf("invalid Zsh must stay rejected: %q", src)
		}
	}
}

// The tree must describe what Zsh runs, not merely parse. A body on the
// following line would make the loop run it zero times under a false
// condition; executing the form shows it runs forever, so the sublist
// belongs to the condition and the body is empty.
func TestParseWhileConditionListShape(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		condLen int
		bodyLen int
		until   bool
		clauses int
	}{
		{name: "one following sublist", src: "while true\nprint x", condLen: 2, bodyLen: 0, clauses: 1},
		{name: "two following sublists", src: "while true\nprint a\nprint b", condLen: 3, bodyLen: 0, clauses: 1},
		{name: "until", src: "until true\nprint x", condLen: 2, bodyLen: 0, until: true, clauses: 1},
		{name: "inside a function", src: "h() { while (( 1 )); print x }", condLen: 2, bodyLen: 0, clauses: 1},
		// The inner loop is one of the outer condition's statements, so the
		// outer holds three and the inner two.
		{name: "nested bare loops", src: "while true\nprint a\nwhile true\nprint b", condLen: 3, bodyLen: 0, clauses: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := parseWithAdapters([]byte(test.src), "t.zsh")
			if err != nil {
				t.Fatalf("must parse: %v", err)
			}

			var clauses []*syntax.WhileClause
			syntax.Walk(tree, func(node syntax.Node) bool {
				if clause, ok := node.(*syntax.WhileClause); ok {
					clauses = append(clauses, clause)
				}
				return true
			})
			if len(clauses) != test.clauses {
				t.Fatalf("found %d while clauses, want %d", len(clauses), test.clauses)
			}

			outer := clauses[0]
			if len(outer.Cond) != test.condLen {
				t.Errorf("condition holds %d statements, want %d", len(outer.Cond), test.condLen)
			}
			if len(outer.Do) != test.bodyLen {
				t.Errorf("body holds %d statements, want %d (the body must be empty)",
					len(outer.Do), test.bodyLen)
			}
			if outer.Until != test.until {
				t.Errorf("Until = %v, want %v", outer.Until, test.until)
			}
		})
	}
}
