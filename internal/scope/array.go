package scope

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type arrayDeclaration struct {
	name     string
	pos      syntax.Pos
	function *syntax.FuncDecl
}

func (m *Map) indexArrayAssignment(assign *syntax.Assign) {
	if assign != nil && assign.Name != nil && assign.Array != nil && assign.Index == nil {
		m.arrays = append(m.arrays, arrayDeclaration{assign.Name.Value, assign.Pos(), m.currentFunc})
	}
}

func (m *Map) indexArrayDeclaration(decl *syntax.DeclClause) {
	if decl.Variant == nil {
		return
	}
	switch decl.Variant.Value {
	case "local", "typeset", "declare":
	default:
		return
	}
	array := false
	for _, assign := range decl.Args {
		if assign == nil {
			continue
		}
		name := ""
		if assign.Name != nil {
			name = assign.Name.Value
		} else if assign.Value != nil && len(assign.Value.Parts) == 1 {
			name = extractLiteral(assign.Value)
		}
		if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "+") {
			if strings.Contains(name[1:], "a") {
				array = name[0] == '-'
			}
			continue
		}
		if name != "" && assign.Index == nil && (array || assign.Array != nil) {
			m.arrays = append(m.arrays, arrayDeclaration{name, assign.Pos(), m.currentFunc})
		}
	}
}

// IsDeclaredArray reports an earlier array declaration in the same lexical
// function or file scope. It deliberately does not infer dynamic caller scope.
func (m *Map) IsDeclaredArray(name string, pos syntax.Pos) bool {
	var function *syntax.FuncDecl
	for _, candidate := range m.functions {
		if candidate.Pos().Offset() <= pos.Offset() && pos.Offset() < candidate.End().Offset() {
			function = candidate
		}
	}
	for _, declaration := range m.arrays {
		if declaration.name == name && declaration.function == function && declaration.pos.Offset() < pos.Offset() {
			return true
		}
	}
	return false
}
