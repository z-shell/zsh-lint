package scope

import (
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
				syntax.Walk(x.Body, func(inner syntax.Node) bool {
					m.Index(inner)
					return false // We handle the descent manually to avoid double-walking
				})
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

			isExport := cmdName == "export"
			isLocal := cmdName == "local" || cmdName == "typeset" || cmdName == "declare"
			isAlias := cmdName == "alias"

			if isExport || isLocal {
				for _, arg := range x.Args[1:] {
					// We only care if they are actually assignments e.g., local foo=bar
					// mvdan/sh parses local foo=bar as a CallExpr with Assign elements in Args.
					// Note: If they just do `local foo`, it's a Word.
					m.processDeclArg(arg, isExport, isLocal)
				}
			} else if isAlias {
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

func (m *Map) processDeclArg(arg *syntax.Word, isExport, isLocal bool) {
	// If it's an assignment like `foo=bar`
	for _, part := range arg.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			// Naive extraction for `foo=bar` vs `foo`
			// mvdan/sh might also hand this over as an explicit syntax.Assign inside the CallExpr
			name := lit.Value
			// if it has an =, split it (simplification for AST extraction)
			for i, c := range lit.Value {
				if c == '=' {
					name = lit.Value[:i]
					break
				}
			}
			if name != "" {
				sym := Symbol{
					Name:     name,
					Kind:     KindVariable,
					Node:     arg,
					Pos:      arg.Pos(),
					Exported: isExport,
					Local:    isLocal,
				}
				m.Add(sym)
				break // Only care about the first literal which holds the var name
			}
		}
	}
}

func (m *Map) processAliasArg(arg *syntax.Word) {
	for _, part := range arg.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			name := lit.Value
			for i, c := range lit.Value {
				if c == '=' {
					name = lit.Value[:i]
					break
				}
			}
			if name != "" && name != "-g" { // ignore global alias flag
				sym := Symbol{
					Name: name,
					Kind: KindAlias,
					Node: arg,
					Pos:  arg.Pos(),
				}
				m.Add(sym)
				break
			}
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
