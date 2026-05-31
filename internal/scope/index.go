package scope

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Index AST walks the tree and populates the Map with all declarations.
func (m *Map) Index(node syntax.Node) {
	if node == nil {
		return
	}

	syntax.Walk(node, func(n syntax.Node) bool {
		switch x := n.(type) {

		// Track function entry/exit for local scoping
		case *syntax.FuncDecl:
			sym := Symbol{
				Name: x.Name.Value,
				Kind: KindFunction,
				Node: x,
				Pos:  x.Pos(),
			}
			m.Add(sym)

			// Descend into the function body with context
			prev := m.currentFunc
			m.currentFunc = x
			if x.Body != nil {
				m.Index(x.Body)
			}
			m.currentFunc = prev
			return false // We already walked the children

		// Handle variable assignments: foo=bar
		case *syntax.Assign:
			if x.Name != nil && x.Name.Value != "" {
				sym := Symbol{
					Name: x.Name.Value,
					Kind: KindVariable,
					Node: x,
					Pos:  x.Pos(),
				}
				m.Add(sym)
			}

		// Handle commands like: export foo=bar OR local foo=bar OR alias l="ls"
		// Handle export, local, declare, typeset
		case *syntax.DeclClause:
			cmdName := ""
			if x.Variant != nil {
				cmdName = x.Variant.Value
			}
			isExport := cmdName == "export"
			isLocal := cmdName == "local" || cmdName == "typeset" || cmdName == "declare"

			for _, assign := range x.Args {
				if assign.Name != nil {
					sym := Symbol{
						Name:     assign.Name.Value,
						Kind:     KindVariable,
						Node:     assign,
						Pos:      assign.Pos(),
						Exported: isExport,
						Local:    isLocal,
					}
					m.Add(sym)
				}
			}
			return false
		case *syntax.CallExpr:
			if len(x.Args) == 0 {
				return true
			}
			cmdName := extractLiteral(x.Args[0])

			isAlias := cmdName == "alias"

			// Under the Bash parser variant, export/local/typeset/declare are
			// DeclClause nodes handled above. CallExpr declaration handling
			// would be unreachable here.
			if isAlias {
				for _, arg := range x.Args[1:] {
					m.processAliasArg(arg)
				}
			}

			// Special case: inline assignments before commands (e.g. FOO=bar some_cmd)
			for _, assign := range x.Assigns {
				if assign.Name != nil {
					sym := Symbol{
						Name: assign.Name.Value,
						Kind: KindVariable,
						Node: assign,
						Pos:  assign.Pos(),
					}
					// Inline env vars are technically global in shell scoping rules
					// (or command-scoped), but we treat them as global for linting declaration existence.
					m.Add(sym)
				}
			}
		}
		return true
	})
}

func (m *Map) processAliasArg(arg *syntax.Word) {
	for _, part := range arg.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			// An alias definition is always of the form name=value. Option flags
			// (-g, -s, -r, ...) carry no '=', so requiring one skips every flag
			// rather than special-casing a single one.
			eq := strings.IndexByte(lit.Value, '=')
			if eq <= 0 {
				// No '=' (an option flag) or an empty name (leading '='); not a
				// definition we should index.
				continue
			}
			sym := Symbol{
				Name: lit.Value[:eq],
				Kind: KindAlias,
				Node: arg,
				Pos:  arg.Pos(),
			}
			m.Add(sym)
			break
		}
	}
}

// extractLiteral attempts to pull the literal text out of a Word node safely.
func extractLiteral(word *syntax.Word) string {
	if word == nil || len(word.Parts) == 0 {
		return ""
	}
	if lit, ok := word.Parts[0].(*syntax.Lit); ok {
		return lit.Value
	}
	return ""
}
