package scope_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/scope"
)

// indexSource parses src and returns its populated scope map.
func indexSource(t *testing.T, src string) *scope.Map {
	t.Helper()
	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sm := scope.NewMap()
	sm.Index(file.AST())
	return sm
}

// TestIndexerAssociativeAttribute pins which declarations record the
// associative-array attribute (#391). Each row's native type was measured
// under zsh 5.9.2 with ${(t)v} after the declaration inside a function.
func TestIndexerAssociativeAttribute(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		assoc bool
	}{
		{"typeset -A with array", "typeset -A v=( a 1 b 2 )", true},
		{"typeset -A bare", "typeset -A v", true},
		{"flag split across args", "typeset -g -A v=( a 1 )", true},
		{"bundled after g", "typeset -gA v", true},
		{"bundled before g", "typeset -Ag v", true},
		{"local -A", "f() { local -A v; }", true},
		{"declare -A", "declare -A v", true},
		{"readonly -A", "readonly -A v=( a 1 )", true},
		{"after option terminator", "typeset -A -- v", true},
		{"with an option that takes an argument", "typeset -A -L 5 v", true},
		{"bundled with an option that takes an argument", "typeset -AL 5 v", true},
		{"plus A then minus A", "typeset +A -A v", true},
		{"minus A then plus A", "typeset -A +A v", false},
		{"normal array flags", "typeset -aU v=( 1 2 )", false},
		{"local -a", "f() { local -a v=( 1 2 3 ); }", false},
		{"plus A removes the attribute", "typeset +A v", false},
		{"option argument is not a flag", "typeset -L 5 v", false},
		{"plain array assignment", "v=( 1 2 3 )", false},
		{"export -A is not an associative declaration", "export -A v", false},
		{"no declaration", "print $v", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := indexSource(t, tc.src)
			if got := sm.MayBeAssociative("v"); got != tc.assoc {
				t.Errorf("MayBeAssociative(%q) after %q = %v, want %v", "v", tc.src, got, tc.assoc)
			}
		})
	}
}

// TestIndexerAssociativeEveryName checks that one -A applies to every
// name the declaration binds.
func TestIndexerAssociativeEveryName(t *testing.T) {
	sm := indexSource(t, "typeset -A a b")
	for _, name := range []string{"a", "b"} {
		if !sm.MayBeAssociative(name) {
			t.Errorf("MayBeAssociative(%q) = false, want true", name)
		}
	}
	if sm.MayBeAssociative("c") {
		t.Errorf("MayBeAssociative(%q) = true for a name never declared", "c")
	}
}

// TestIndexerAssociativeIsSticky pins the conservative, whole-file answer:
// once any declaration in the file makes a name associative, a later plain
// reassignment or a redeclaration as a normal array does not clear it, so a
// consuming rule stays silent rather than risking a false reject.
func TestIndexerAssociativeIsSticky(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"reassigned as an array", "typeset -A v=( a 1 )\nv=( 1 2 3 )"},
		{"unset and redeclared as a normal array", "typeset -A v=( a 1 )\nunset v\nlocal -a v=( 1 2 3 )"},
		{"declared associative after a normal array", "local -a v=( 1 )\ntypeset -A v"},
		{"associative only inside a function", "f() { local -A v; }\ng() { local -a v; }"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !indexSource(t, tc.src).MayBeAssociative("v") {
				t.Errorf("MayBeAssociative(%q) = false after %q, want true", "v", tc.src)
			}
		})
	}
}

// TestIndexerAssociativeSymbol checks the attribute on the recorded Symbol
// itself, per declaration and scope, alongside the file-wide query.
func TestIndexerAssociativeSymbol(t *testing.T) {
	sm := indexSource(t, "typeset -A g=( a 1 )\nplain=1\nf() { local -A l; }")
	if !sm.Globals["g"].Associative {
		t.Errorf("Globals[%q].Associative = false, want true", "g")
	}
	if sm.Globals["plain"].Associative {
		t.Errorf("Globals[%q].Associative = true, want false", "plain")
	}
	found := false
	for _, locals := range sm.Locals {
		if sym, ok := locals["l"]; ok {
			found = true
			if !sym.Associative {
				t.Errorf("local %q Associative = false, want true", "l")
			}
		}
	}
	if !found {
		t.Fatalf("local %q not recorded", "l")
	}
}

// TestIndexerAssociativeUnknownName checks that a -A declaration whose
// name the indexer cannot read (quoted or computed) makes every name a
// possible associative array: the declared name is unknown, so no name can
// be ruled out.
func TestIndexerAssociativeUnknownName(t *testing.T) {
	for _, src := range []string{`typeset -A "$n"`, "typeset -A 'v'", `typeset -A "$n"x`, `typeset -A v"$n"`, `typeset -A a "$n"`} {
		sm := indexSource(t, src)
		for _, name := range []string{"v", "other"} {
			if !sm.MayBeAssociative(name) {
				t.Errorf("MayBeAssociative(%q) = false after %q, want true", name, src)
			}
		}
	}
	// An unreadable name without -A says nothing about any other name.
	if indexSource(t, `typeset "$n"`).MayBeAssociative("v") {
		t.Errorf("an unreadable non-associative declaration made %q associative", "v")
	}
}
