package scope

import (
	"strings"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

type numericDeclaration struct {
	name     string
	pos      syntax.Pos
	function *syntax.FuncDecl
	subst    *syntax.CmdSubst
}

type unsetName struct {
	name     string
	function *syntax.FuncDecl
	subst    *syntax.CmdSubst
}

func (m *Map) indexNumericDeclaration(decl *syntax.DeclClause) {
	if decl.Variant == nil {
		return
	}
	numeric := false
	switch decl.Variant.Value {
	case "integer", "float":
		numeric = true
	case "local", "typeset", "declare":
	default:
		return
	}
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
			if strings.ContainsAny(name[1:], "iEF") {
				numeric = name[0] == '-'
			}
			continue
		}
		if name != "" && numeric && assign.Index == nil && assign.Array == nil {
			m.numerics = append(m.numerics, numericDeclaration{name, assign.Pos(), m.currentFunc, m.enclosingSubst(assign.Pos())})
		}
	}
}

func (m *Map) indexUnset(call *syntax.CallExpr) {
	if len(call.Args) == 0 || extractLiteral(call.Args[0]) != "unset" {
		return
	}
	for _, arg := range call.Args[1:] {
		if name := staticText(arg); name != "" && !strings.HasPrefix(name, "-") {
			m.unsets = append(m.unsets, unsetName{strings.SplitN(name, "[", 2)[0], m.currentFunc, m.enclosingSubst(call.Pos())})
		}
	}
}

// staticText returns the text of a word made only of literal, single-quoted
// and double-quoted literal parts, so `unset 'n'` and `unset "n"` name n.
func staticText(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	var text strings.Builder
	for _, part := range word.Parts {
		switch x := part.(type) {
		case *syntax.Lit:
			text.WriteString(x.Value)
		case *syntax.SglQuoted:
			text.WriteString(x.Value)
		case *syntax.DblQuoted:
			for _, inner := range x.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return ""
				}
				text.WriteString(lit.Value)
			}
		default:
			return ""
		}
	}
	return text.String()
}

// IsDeclaredNumeric reports an earlier integer or float declaration (integer,
// float, or typeset, local or declare with -i, -E or -F) in the same lexical
// function or file scope. Such a parameter always expands to a number, so it
// can never be empty unless it is unset; any unset of the name in that scope
// makes the answer false. A declaration or unset inside a command
// substitution applies only within that substitution. It does not infer
// dynamic caller scope.
func (m *Map) IsDeclaredNumeric(name string, pos syntax.Pos) bool {
	function := m.enclosingFunction(pos)
	for _, unset := range m.unsets {
		if unset.name == name && unset.function == function && reaches(unset.subst, pos) {
			return false
		}
	}
	for _, declaration := range m.numerics {
		if declaration.name == name && declaration.function == function && declaration.pos.Offset() < pos.Offset() &&
			reaches(declaration.subst, pos) {
			return true
		}
	}
	return false
}

// enclosingSubst returns the innermost command substitution that holds pos,
// or nil. Its body runs in a subshell, so a declaration or unset there does
// not reach the enclosing scope (#252).
func (m *Map) enclosingSubst(pos syntax.Pos) *syntax.CmdSubst {
	var subst *syntax.CmdSubst
	for _, candidate := range m.substs {
		if candidate.Pos().Offset() <= pos.Offset() && pos.Offset() < candidate.End().Offset() {
			subst = candidate
		}
	}
	return subst
}

// reaches reports whether a declaration or unset made in subst, or outside
// every command substitution when subst is nil, applies at pos.
func reaches(subst *syntax.CmdSubst, pos syntax.Pos) bool {
	return subst == nil || subst.Pos().Offset() <= pos.Offset() && pos.Offset() < subst.End().Offset()
}

func (m *Map) enclosingFunction(pos syntax.Pos) *syntax.FuncDecl {
	var function *syntax.FuncDecl
	for _, candidate := range m.functions {
		if candidate.Pos().Offset() <= pos.Offset() && pos.Offset() < candidate.End().Offset() {
			function = candidate
		}
	}
	return function
}
