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
// any construct in the file may make associative, in any scope and at any
// position.
//
// The answer is deliberately file-wide. The attribute sticks through a
// plain reassignment, but unset clears it and a later declaration can
// give the same name another type, so a name's type depends on position
// and execution path, which this index does not model.
//
// It is also file-local: a global declared associative in another file,
// such as a plugin's entry point, is invisible here. A false answer
// therefore means only that nothing in this file makes the name
// associative. A rule must not reject a subscript on that alone; it also
// needs evidence that the name is not associative, such as a declaration
// of another type in scope, or project-wide context.
func (m *Map) MayBeAssociative(name string) bool {
	return m.anyAssociative || m.associative[name] || specialAssociative[name]
}
