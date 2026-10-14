package rules

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// resolvedCommand is the command Zsh runs for a simple command, after its
// precommand modifiers (zshmisc, Precommand Modifiers:
// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Precommand-Modifiers).
type resolvedCommand struct {
	// name is the static command name after quote removal, or empty when it
	// is dynamic, missing, or a modifier option Zsh rejects.
	name string
	// args are the words after name, retaining their original nodes.
	args []*syntax.Word
	// external reports a name after the command modifier. With the default
	// posix_builtins off, Zsh then runs only an external command, never a
	// builtin, function or alias.
	external bool
	// exec reports the exec modifier: the shell is replaced, so the command
	// never returns to the caller.
	exec bool
	// optionArgs are separate option-argument words a modifier consumed, such
	// as NAME in exec -a NAME. They are expanded like arguments.
	optionArgs []*syntax.Word
}

// effectiveCommand resolves the builtin, function or external command name
// after the precommand modifiers -, builtin, exec, noglob and a leading
// nocorrect, and returns the argument words after that name. A name after the
// command modifier is reported as unresolved, because Zsh runs only an
// external command there; external-only rules use resolveCommand. An unresolved
// name is empty, but its arguments are still returned for argument-only rules.
func effectiveCommand(call *syntax.CallExpr) (string, []*syntax.Word) {
	resolved := resolveCommand(call)
	if resolved.external {
		return "", resolved.args
	}
	return resolved.name, resolved.args
}

// resolveCommand strips the precommand modifiers Zsh applies before running a
// command. nocorrect is a reserved word, recognized only unquoted and only as
// the first word; the other modifiers are recognized after quote removal and in
// any order. command ends the modifiers: what follows is an external command.
func resolveCommand(call *syntax.CallExpr) resolvedCommand {
	var resolved resolvedCommand
	if call == nil {
		return resolved
	}
	args := call.Args
	for len(args) > 0 && isBareNocorrect(args[0]) {
		args = args[1:]
	}
	for len(args) > 0 {
		word, ok := staticDeclarationWord(args[0])
		if !ok {
			resolved.args = args[1:]
			return resolved
		}
		switch word {
		case "-", "builtin", "noglob":
			args = args[1:]
		case "exec":
			resolved.exec = true
			var valid bool
			var names []*syntax.Word
			args, names, valid = skipExecOptions(args[1:])
			resolved.optionArgs = append(resolved.optionArgs, names...)
			if !valid {
				resolved.args = args
				return resolved
			}
		case "command":
			resolved.external = true
			var valid bool
			if args, valid = skipCommandOptions(args[1:]); !valid || len(args) == 0 {
				resolved.args = args
				return resolved
			}
			name, ok := staticDeclarationWord(args[0])
			if ok {
				resolved.name = name
			}
			resolved.args = args[1:]
			return resolved
		default:
			resolved.name = word
			resolved.args = args[1:]
			return resolved
		}
	}
	return resolved
}

// isBareNocorrect reports the reserved word nocorrect: a single unquoted
// literal, since quoting a reserved word makes it an ordinary command name.
func isBareNocorrect(word *syntax.Word) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	literal, ok := word.Parts[0].(*syntax.Lit)
	return ok && literal.Value == "nocorrect"
}

// skipExecOptions consumes exec's -c, -l and -a NAME options, in clusters, up
// to "--" or the first word that is not an option. Letters after a in the same
// word are the name; otherwise the next word is. A lone "-" and a dynamic word are
// left for the caller, which strips the dash modifier and treats a dynamic word
// as the command. valid is false for an unknown option letter, which Zsh
// rejects. An -a without a name leaves nothing to run. names holds each
// separate NAME word consumed by -a.
func skipExecOptions(args []*syntax.Word) (rest, names []*syntax.Word, valid bool) {
	for len(args) > 0 {
		word, ok := staticDeclarationWord(args[0])
		if !ok {
			return args, names, true
		}
		if word == "--" {
			return args[1:], names, true
		}
		if len(word) < 2 || word[0] != '-' {
			return args, names, true
		}
		args = args[1:]
		for i := 1; i < len(word); i++ {
			switch word[i] {
			case 'c', 'l':
			case 'a':
				if i+1 == len(word) && len(args) > 0 {
					names = append(names, args[0])
					args = args[1:]
				}
				i = len(word)
			default:
				return args, names, false
			}
		}
	}
	return args, names, true
}

// skipCommandOptions consumes command's -p option, in clusters, up to "--" or
// the first word that is not an option. valid is false for -v and -V, which
// print a description instead of running the command, and for unknown option
// letters. A dynamic word is left for the caller as the command.
func skipCommandOptions(args []*syntax.Word) (rest []*syntax.Word, valid bool) {
	for len(args) > 0 {
		word, ok := staticDeclarationWord(args[0])
		if !ok {
			return args, true
		}
		if word == "--" {
			return args[1:], true
		}
		if len(word) < 2 || word[0] != '-' {
			return args, true
		}
		if strings.Trim(word[1:], "p") != "" {
			return args[1:], false
		}
		args = args[1:]
	}
	return args, true
}
