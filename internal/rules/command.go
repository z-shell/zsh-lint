package rules

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// effectiveCommand resolves a static command name after builtin/command
// prefixes and returns the argument words after that name. An unresolved name
// is empty, but its arguments are still returned for argument-only rules.
func effectiveCommand(call *syntax.CallExpr) (string, []*syntax.Word) {
	if call == nil {
		return "", nil
	}
	for i := 0; i < len(call.Args); i++ {
		name, _ := commandLiteral(call.Args[i])
		name = strings.TrimPrefix(name, `\`)
		switch name {
		case "builtin":
			continue
		case "command":
			if i+1 < len(call.Args) {
				if option, _ := commandLiteral(call.Args[i+1]); option == "-p" {
					i++
				}
			}
			continue
		default:
			return name, call.Args[i+1:]
		}
	}
	return "", nil
}

func commandLiteral(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var name strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			name.WriteString(value.Value)
		case *syntax.SglQuoted:
			if value.Dollar {
				return "", false
			}
			name.WriteString(value.Value)
		case *syntax.DblQuoted:
			text, ok := commandLiteral(&syntax.Word{Parts: value.Parts})
			if !ok {
				return "", false
			}
			name.WriteString(text)
		default:
			return "", false
		}
	}
	return name.String(), true
}
