package scope

import (
	"mvdan.cc/sh/v3/syntax"
)

// Kind represents what type of entity is being tracked in scope.
type Kind int

const (
	KindUnknown Kind = iota
	KindVariable
	KindFunction
	KindAlias
)

// Symbol represents an entity declared in the script.
type Symbol struct {
	Name     string
	Kind     Kind
	Node     syntax.Node // The node where it was declared
	Pos      syntax.Pos  // The start position of the declaration
	Exported bool        // True if 'export' was used
	Local    bool        // True if 'local' or 'typeset' was used inside a function
	// Associative is true when this declaration gave the name the
	// associative-array attribute (-A). A name's type can change within a
	// file, and other constructs create associative arrays too, so a rule
	// should ask Map.MayBeAssociative, not one Symbol.
	Associative bool
}

// Map tracks all declarations found in Pass 1 of the analysis.
// Shell scope is notoriously messy; variables can be dynamically scoped
// and "hoisted" depending on execution path. For static analysis, we index
// symbols globally and loosely track function-local vs global.
type Map struct {
	// Global declarations indexed by name
	Globals map[string]Symbol

	// Local declarations grouped by the function node that owns them
	Locals map[*syntax.FuncDecl]map[string]Symbol

	// The current function context during Pass 1 (nil if at top-level)
	currentFunc *syntax.FuncDecl

	// associative holds every name a construct in the file made
	// associative, in any scope; anyAssociative is set when such a
	// construct names something the indexer cannot read.
	associative    map[string]bool
	anyAssociative bool
}

// NewMap creates an empty scope map.
func NewMap() *Map {
	return &Map{
		Globals: make(map[string]Symbol),
		Locals:  make(map[*syntax.FuncDecl]map[string]Symbol),
	}
}

// Add records a new symbol into the scope map.
// If local is true and we are inside a function, it binds to that function.
// Otherwise, it binds globally.
func (m *Map) Add(sym Symbol) {
	if sym.Local && m.currentFunc != nil {
		if _, ok := m.Locals[m.currentFunc]; !ok {
			m.Locals[m.currentFunc] = make(map[string]Symbol)
		}
		m.Locals[m.currentFunc][sym.Name] = sym
	} else {
		m.Globals[sym.Name] = sym
	}
}

// IsDeclared checks if a variable name has been seen in either global
// scope or the provided local function context.
func (m *Map) IsDeclared(name string, context *syntax.FuncDecl) bool {
	if context != nil {
		if locals, ok := m.Locals[context]; ok {
			if _, exists := locals[name]; exists {
				return true
			}
		}
	}
	_, exists := m.Globals[name]
	return exists
}

// MayBeAssociative reports whether name may be an associative array at
// some point in this file: a special parameter that is associative in Zsh
// itself (options, commands, parameters, terminfo and the like), or a name
// a construct the index recognises may make associative, in any scope and
// at any position.
//
// The answer is advisory. A true answer is a reason to stay silent; a
// false answer is not a reason to reject. A rule that would reject a
// subscript only an associative array accepts must also find a
// declaration in scope that gives the name another type (a scalar,
// integer, float or normal array), because the index cannot see every way
// a name becomes associative:
//
//   - Other files: a global declared associative in a plugin's entry point
//     is invisible to the index of an autoloaded function file.
//   - Code built from strings and run later: an alias expanded inside eval
//     (alias T='typeset -A'; eval 'T v'), a function body assigned through
//     functions[name]=, a trap body, or a sourced file.
//   - Code whose text is only known at run time: eval or emulate -c of a
//     computed body, and ${(P)...} indirection (these make every name
//     possibly associative).
//   - Rare command forms the index does not read: a declaration behind
//     the - precommand modifier (- typeset -A v), or behind command when
//     POSIX_BUILTINS is set (command typeset -A v).
//   - Position and path: the attribute sticks through a plain
//     reassignment, but unset clears it and a later declaration can give
//     the name another type, so the answer is file-wide, not positional.
func (m *Map) MayBeAssociative(name string) bool {
	return m.anyAssociative || m.associative[name] || specialAssociative[name]
}
