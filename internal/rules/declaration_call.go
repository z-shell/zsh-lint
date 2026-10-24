package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// ruleDeclaration reads the ordinary builtin interface as declaration operands
// without changing the parser's tree or inventing array assignments. In that
// interface every argument keeps its original expansion and quoting semantics.
// See https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-typeset.
func ruleDeclaration(node syntax.Node) *syntax.DeclClause {
	switch value := node.(type) {
	case *syntax.DeclClause:
		return value
	case *syntax.CallExpr:
		name, words := effectiveCommand(value)
		switch name {
		case "local", "typeset", "declare", "integer", "float", "readonly", "export":
		default:
			return nil
		}
		command := value.Args[len(value.Args)-len(words)-1]
		args := make([]*syntax.Assign, 0, len(words))
		for _, word := range words {
			if word != nil {
				args = append(args, &syntax.Assign{Naked: true, Value: word})
			}
		}
		// The optional third -T operand is separator data, never a parameter.
		scan := scanDeclarationOptions(name, args)
		if separator := scan.tiedParameterSeparator(args); separator != nil {
			args = args[:scan.firstOperand+2]
		}
		return &syntax.DeclClause{
			Variant: &syntax.Lit{Value: name, ValuePos: command.Pos(), ValueEnd: command.End()},
			Args:    args,
		}
	default:
		return nil
	}
}

// declarationWordName accepts a statically known name even when the value is
// expanded. It never evaluates an expansion to manufacture a parameter name.
func declarationWordName(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	name, assigned := declarationNamePrefix(word.Parts, false)
	if !assigned {
		var static bool
		name, static = staticDeclarationWord(word)
		if !static {
			return "", false
		}
	}
	return name, name == "0" || syntax.ValidName(name)
}

func declarationNamePrefix(parts []syntax.WordPart, doubleQuoted bool) (string, bool) {
	var prefix strings.Builder
	for _, part := range parts {
		if quoted, ok := part.(*syntax.DblQuoted); ok {
			name, assigned := declarationNamePrefix(quoted.Parts, true)
			prefix.WriteString(name)
			if assigned {
				return prefix.String(), true
			}
			if _, static := staticDeclarationWord(&syntax.Word{Parts: quoted.Parts}); !static {
				return "", false
			}
			continue
		}
		text, static := staticDeclarationWord(&syntax.Word{Parts: []syntax.WordPart{part}})
		if literal, ok := part.(*syntax.Lit); ok && doubleQuoted {
			text, static = removeDoubleQuotedBackslashEscapes(literal.Value), true
		}
		if !static {
			return "", false
		}
		before, _, assigned := strings.Cut(text, "=")
		prefix.WriteString(before)
		if assigned {
			return prefix.String(), true
		}
	}
	return prefix.String(), false
}

func declarationWordAssigns(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	_, assigned := declarationNamePrefix(word.Parts, false)
	return assigned
}

// Only quoted static text and plain unquoted name/value bytes prove that an
// ordinary operand survives expansion. An empty array product or a null glob
// can remove even a name= prefix; it must not hide a later global write.
func declarationProvesLocal(assign *syntax.Assign) bool {
	if assign.Name != nil {
		return true
	}
	if assign.Value == nil {
		return false
	}
	for _, part := range assign.Value.Parts {
		if literal, ok := part.(*syntax.Lit); ok {
			for _, char := range literal.Value {
				plain := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
					char >= '0' && char <= '9' || strings.ContainsRune("_=./:-", char)
				if !plain {
					return false
				}
			}
		}
	}
	_, static := staticDeclarationWord(assign.Value)
	return static
}
