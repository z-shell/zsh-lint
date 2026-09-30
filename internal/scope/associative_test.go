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

// TestIndexerAssociativeReviewShapes pins the shapes the first review of
// #574 found (each native type measured under zsh 5.9.2): a flag or a name
// the parser does not bind as an assignment, and a declaration command the
// parser reads as an ordinary call.
func TestIndexerAssociativeReviewShapes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		v, w bool
	}{
		{"escaped name", `typeset -A \v`, true, false},
		{"escaped name then a name", `typeset -A \v w`, true, true},
		{"single-quoted name", `typeset -A 'v'`, true, false},
		{"double-quoted name then a name", `typeset -A "v" w`, true, true},
		{"brace-expanded names (any name)", `typeset -A {v,w}`, true, true},
		{"brace-expanded one name (any name)", `typeset -A v{,}`, true, true},
		{"single-quoted flag", `typeset '-A' v`, true, false},
		{"double-quoted flag", `typeset "-A" v`, true, false},
		{"escaped flag", `typeset \-A v`, true, false},
		{"computed flag", `typeset $opt v`, true, false},
		{"quoted computed flag", `typeset "$opt" v`, true, false},
		{"builtin precommand", "builtin typeset -A v", true, false},
		{"builtin precommand bundled", "builtin typeset -gA v", true, false},
		{"noglob precommand", "noglob typeset -A v", true, false},
		{"nocorrect precommand", "nocorrect typeset -A v", true, false},
		{"stacked precommands", "noglob builtin typeset -A v", true, false},
		{"builtin local", "f() { builtin local -A v; }", true, false},
		{"escaped command name", `\typeset -A v`, true, false},
		{"single-quoted command name", `'typeset' -A v`, true, false},
		{"double-quoted command name", `"typeset" -A v`, true, false},
		{"escape inside the command name", `typ\eset -A v`, true, false},
		{"private", "f() { private -A v; }", true, false},
		{"private normal array", "f() { private -a v; }", false, false},
		{"builtin normal array", "builtin typeset -a v", false, false},
		{"command is not a precommand for builtins", "command typeset -A v", false, false},
		{"numeric option argument", "typeset -L 5 v", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := indexSource(t, tc.src)
			if got := sm.MayBeAssociative("v"); got != tc.v {
				t.Errorf("MayBeAssociative(%q) after %q = %v, want %v", "v", tc.src, got, tc.v)
			}
			if got := sm.MayBeAssociative("w"); got != tc.w {
				t.Errorf("MayBeAssociative(%q) after %q = %v, want %v", "w", tc.src, got, tc.w)
			}
		})
	}
}

// TestIndexerAssociativeZeroMap checks that indexing into a zero-value Map
// does not panic on an associative declaration.
func TestIndexerAssociativeZeroMap(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("typeset -A v\n"), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sm := &scope.Map{Globals: map[string]scope.Symbol{}}
	sm.Index(file.AST())
	if !sm.MayBeAssociative("v") {
		t.Errorf("MayBeAssociative(%q) = false on a zero-value map", "v")
	}
}

// TestIndexerAssociativeUnknownName checks that a -A declaration whose
// name the indexer cannot read (quoted or computed) makes every name a
// possible associative array: the declared name is unknown, so no name can
// be ruled out.
func TestIndexerAssociativeUnknownName(t *testing.T) {
	for _, src := range []string{`typeset -A "$n"`, `typeset -A "$n"x`, `typeset -A v"$n"`, `typeset -A a "$n"`} {
		t.Run(src, func(t *testing.T) {
			sm := indexSource(t, src)
			for _, name := range []string{"v", "other"} {
				if !sm.MayBeAssociative(name) {
					t.Errorf("MayBeAssociative(%q) = false after %q, want true", name, src)
				}
			}
		})
	}
	// An unreadable name without -A says nothing about any other name.
	if indexSource(t, `typeset "$n"`).MayBeAssociative("v") {
		t.Errorf("an unreadable non-associative declaration made %q associative", "v")
	}
}

// TestIndexerAssociativeOtherConstructs pins the constructs other than a
// declaration that create an associative array, each measured under
// zsh 5.9.2, and their near-misses that do not.
func TestIndexerAssociativeOtherConstructs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		v, w bool
		any  bool // every name (checked with "other")
	}{
		{"zparseopts -A", "zparseopts -A v a b", true, false, false},
		{"zparseopts -A after -D", "zparseopts -D -A v a b", true, false, false},
		{"zparseopts -A bundled with name", "zparseopts -Av a b", true, false, false},
		{"zparseopts -M -A", "zparseopts -M -A v a b", true, false, false},
		{"zparseopts -a is a normal array", "zparseopts -a v a b", false, false, false},
		{"zparseopts spec after operands is not an option", "zparseopts a -A v", false, false, false},
		{"zparseopts computed name", `zparseopts -A "$n" a`, false, false, true},
		{"zstat -H", "zstat -H v /", true, false, false},
		{"zstat -H bundled", "zstat -nH v /", true, false, false},
		{"zstat -H after a selector", "zstat +mode -H v /", true, false, false},
		{"stat -H", "stat -H v /", true, false, false},
		{"zstat without -H", "zstat -A v /", false, false, false},
		{"ztie", "ztie -d db/gdbm -f db v", true, false, false},
		{"AA flag with =", ": ${(AA)=v::=a 1 b 2}", true, false, false},
		{"AA flag with more flags", ": ${(AAo)=v::=a 1}", true, false, false},
		{"AA flag in a declaration value", "typeset x=${(AA)=v::=a 1}", true, false, false},
		{"two AA flags", ": ${(AA)=v::=a 1}; : ${(AA)=w::=b 2}", true, true, false},
		{"single A flag is a normal array", ": ${(A)v::=a 1}", false, false, false},
		{"AA flag without assignment", "print ${(AA)v}", false, false, false},
		{"eval of a literal declaration", "eval 'typeset -A v'", true, false, false},
		{"eval of a double-quoted declaration", `eval "typeset -A v"`, true, false, false},
		{"eval of split words", "eval typeset -A v", true, false, false},
		{"eval of a normal array", "eval 'typeset -a v'", false, false, false},
		{"eval of a computed body", "eval $x", false, false, true},
		{"builtin eval", "builtin eval 'typeset -A v'", true, false, false},
		// Precision rows: each pins a case where the answer must stay
		// narrow, so a conservative fallback cannot widen unnoticed.
		{"numeric option argument under -A marks only the name", "typeset -A -L 5 v", true, false, false},
		{"zstat +H selects an element, not a name", "zstat +H -H v /", true, false, false},
		{"zparseopts +A is not -A", "zparseopts +A v a", false, false, false},
		{"ANSI-C quoted name may be any name", "typeset -A $'v'", true, true, true},
		{"backslash in a double-quoted name may be any name", `typeset -A "\v"`, true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := indexSource(t, tc.src)
			for _, c := range []struct {
				name string
				want bool
			}{{"v", tc.v || tc.any}, {"w", tc.w || tc.any}, {"other", tc.any}} {
				if got := sm.MayBeAssociative(c.name); got != c.want {
					t.Errorf("MayBeAssociative(%q) after %q = %v, want %v", c.name, tc.src, got, c.want)
				}
			}
		})
	}
}

// TestIndexerAssociativeSpecials checks the special parameters Zsh itself
// makes associative, with no declaration in the file.
func TestIndexerAssociativeSpecials(t *testing.T) {
	sm := indexSource(t, "print ok")
	for _, name := range []string{"options", "commands", "parameters", "functions", "aliases", "terminfo", "mapfile", "widgets"} {
		if !sm.MayBeAssociative(name) {
			t.Errorf("MayBeAssociative(%q) = false for a special associative parameter", name)
		}
	}
	for _, name := range []string{"path", "fpath", "argv", "pipestatus", "v"} {
		if sm.MayBeAssociative(name) {
			t.Errorf("MayBeAssociative(%q) = true for a parameter that is not associative", name)
		}
	}
}
