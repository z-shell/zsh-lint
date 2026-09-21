package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Alternate Forms For Complex Commands requires an alternate-form test to be
// "suitably delimited, such as by `[[ ... ]]` or `(( ... ))`, else the end of
// the test will not be recognized". A `{ ... }` group written after `&&` or
// `||` is therefore another element of the condition list, not the body.
//
// The adapter used to take that group as the body opener, so an ordinary
// `if ... ; then` whose condition holds a brace group failed to parse whenever
// the same file also contained an alternate-form construct. The adapter only
// runs when a file has an alternate form to fix, which is why each source here
// pairs the two.
func TestAlternateIfConditionBraceGroup(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			"ordinary if after an alternate if",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && { true; }; then\n  :\nfi\n",
		},
		{
			"ordinary if before an alternate if",
			"if [[ -n x ]] && { true; }; then\n  :\nfi\nif [[ -n x ]] {\n  y=1\n}\n",
		},
		{
			"alternate arithmetic if",
			"if (( 1 )) {\n  y=1\n}\nif [[ -n x ]] && { true; }; then\n  :\nfi\n",
		},
		{
			"elif condition",
			"if [[ -n x ]] {\n  y=1\n}\nif false; then\n  :\nelif [[ -n x ]] && { true; }; then\n  :\nfi\n",
		},
		{
			"while condition",
			"if [[ -n x ]] {\n  y=1\n}\nwhile [[ -n \"\" ]] && { false; }; do\n  break\ndone\n",
		},
		{
			"or connective",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] || { true; }; then\n  :\nfi\n",
		},
		{
			"negated group",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && ! { true; }; then\n  :\nfi\n",
		},
		{
			"two condition groups",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && { true; } && { true; }; then\n  :\nfi\n",
		},
		{
			"nested condition group",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && { { true; }; }; then\n  :\nfi\n",
		},
		{
			"comment before the group",
			"if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && # c\n{ true; }; then\n  :\nfi\n",
		},
		{
			"elif chain with an else",
			"if [[ -n x ]] {\n  y=1\n}\nif false; then\n  :\nelif [[ -n x ]] && { true; }; then\n  :\nelse\n  :\nfi\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The alternate form itself must keep working: these are the shapes the
// adapter exists for, and a fix that suppressed the body brace would break
// them.
func TestAlternateIfStillRecognized(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"plain test", "if [[ -n x ]] {\n  print a\n}\n"},
		{"arithmetic test", "if (( 1 )) {\n  print a\n}\n"},
		{"second test after and", "if [[ -n x ]] && [[ -n y ]] { print b }\n"},
		{"second test after or", "if [[ -n x ]] || [[ -n y ]] { print b }\n"},
		{"negated second test", "if [[ -n x ]] && ! [[ -n y ]] { print b }\n"},
		{"elif", "if [[ -n x ]] {\n  :\n} elif [[ -n y ]] {\n  :\n}\n"},
		{"else", "if [[ -n x ]] {\n  :\n} else {\n  :\n}\n"},
		{"while", "while (( 0 )) {\n  break\n}\n"},
		{"nested", "if [[ -n x ]] {\n  if [[ -n y ]] {\n    :\n  }\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// Native Zsh rejects a condition that ends in a brace group with no body:
// `if [[ -n $b ]] && { print body }` is a parse error, because the group is
// part of the condition list and the `if` then has no body at all.
func TestAlternateIfConditionGroupWithoutBody(t *testing.T) {
	src := "if [[ -n x ]] && { print body }\n"
	if _, err := parseWithAdapters([]byte(src), "t.zsh"); err == nil {
		t.Fatalf("invalid Zsh accepted: %q", src)
	}
}

// Parsing is not enough: the condition brace group must stay in the condition.
// A regression that moved it into the body would still parse, so assert the
// shape the issue specifies -- an ordinary IfClause whose condition is a
// BinaryCmd `&&` joining a TestClause and a Block.
func TestAlternateIfConditionBraceGroupShape(t *testing.T) {
	src := "if [[ -n x ]] {\n  y=1\n}\nif [[ -n x ]] && { true; }; then\n  :\nfi\n"
	file, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}

	var clauses []*syntax.IfClause
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if clause, ok := node.(*syntax.IfClause); ok {
			clauses = append(clauses, clause)
		}
		return true
	})
	if len(clauses) != 2 {
		t.Fatalf("%d if clauses, want 2", len(clauses))
	}

	ordinary := clauses[1]
	if len(ordinary.Cond) != 1 {
		t.Fatalf("condition holds %d statements, want 1", len(ordinary.Cond))
	}
	binary, ok := ordinary.Cond[0].Cmd.(*syntax.BinaryCmd)
	if !ok {
		t.Fatalf("condition is %T, want *syntax.BinaryCmd", ordinary.Cond[0].Cmd)
	}
	if binary.Op != syntax.AndStmt {
		t.Errorf("condition operator is %v, want &&", binary.Op)
	}
	if _, ok := binary.X.Cmd.(*syntax.TestClause); !ok {
		t.Errorf("condition left side is %T, want *syntax.TestClause", binary.X.Cmd)
	}
	if _, ok := binary.Y.Cmd.(*syntax.Block); !ok {
		t.Errorf("condition right side is %T, want *syntax.Block", binary.Y.Cmd)
	}
}

// `if [[ -n $b ]] && { print cond; } { print body }` runs under `zsh -f`,
// printing both words: the first group belongs to the condition and the second
// brace is the body. Assert that split, so the body cannot silently become the
// condition group.
func TestAlternateIfConditionGroupThenBody(t *testing.T) {
	src := "if [[ -n x ]] && { print COND; } { print BODY }\n"
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
	if len(clause.Then) != 1 {
		t.Fatalf("body holds %d statements, want 1", len(clause.Then))
	}
	body := clause.Then[0]
	got := src[body.Pos().Offset():body.End().Offset()]
	if got != "print BODY" {
		t.Errorf("body is %q, want %q", got, "print BODY")
	}
	if _, ok := clause.Cond[0].Cmd.(*syntax.BinaryCmd); !ok {
		t.Errorf("condition is %T, want *syntax.BinaryCmd", clause.Cond[0].Cmd)
	}
}

// scanAlternateConditionBrace decides body-or-condition, so test it directly:
// a returned offset is the body opener only when it holds a `{`.
func TestScanAlternateConditionBrace(t *testing.T) {
	// Each source starts `if [[ -n x ]]`, so the scan begins at offset 13,
	// just past the condition's closing `]]`.
	const condEnd = 13
	for _, tc := range []struct {
		name string
		src  string
		body bool
	}{
		{"body directly after the test", "if [[ -n x ]] { body }\n", true},
		{"body after a second test", "if [[ -n x ]] && [[ -n y ]] { body }\n", true},
		{"body after a second arithmetic test", "if [[ -n x ]] && (( 1 )) { body }\n", true},
		{"condition group, no body", "if [[ -n x ]] && { true; }; then :; fi\n", false},
		{"condition group after or", "if [[ -n x ]] || { true; }; then :; fi\n", false},
		{"negated condition group", "if [[ -n x ]] && ! { true; }; then :; fi\n", false},
		{"two condition groups", "if [[ -n x ]] && { true; } && { true; }; then :; fi\n", false},
		{"condition group then body", "if [[ -n x ]] && { print cond; } { print body }\n", true},
		{"plain command after the connective", "if [[ -n x ]] && true; then :; fi\n", false},
		{"unterminated condition group", "if [[ -n x ]] && { true;\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scanAlternateConditionBrace([]byte(tc.src), condEnd, false)
			isBody := got >= 0 && got < len(tc.src) && tc.src[got] == '{'
			if isBody != tc.body {
				t.Fatalf("offset %d (body=%v), want body=%v", got, isBody, tc.body)
			}
		})
	}
}

// Sources native Zsh rejects, kept as .txt so the repository-wide `zsh -n`
// gate never sees them.
func TestAlternateIfConditionBraceInvalidFixtures(t *testing.T) {
	for _, name := range []string{
		"invalid-284-condition-group-without-body.txt",
	} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, err := parseWithAdapters(src, name); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", src)
			}
		})
	}
}
