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
	// file, so a rule should ask Map.MayBeAssociative, not one Symbol.
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

	// associative holds every name any declaration in the file made
	// associative, in any scope; anyAssociative is set by an associative
	// declaration whose name the indexer cannot read.
	associative    map[string]bool
	anyAssociative bool
}

// NewMap creates an empty scope map.
func NewMap() *Map {
	return &Map{
		Globals:     make(map[string]Symbol),
		Locals:      make(map[*syntax.FuncDecl]map[string]Symbol),
		associative: make(map[string]bool),
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

// MayBeAssociative reports whether any declaration in the file may make
// name an associative array, in any scope and at any position.
//
// The answer is deliberately file-wide. The attribute sticks through a
// plain reassignment, but unset clears it and a later declaration can
// give the same name another type, so a name's type depends on position
// and execution path, which this index does not model. A rule that
// rejects source only when this returns false therefore never rejects
// code that is valid on an associative array, at the cost of staying
// silent on a name that is associative somewhere else in the file.
func (m *Map) MayBeAssociative(name string) bool {
	return m.anyAssociative || m.associative[name]
}
