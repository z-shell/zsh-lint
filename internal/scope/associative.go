package scope

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/parse"
	"mvdan.cc/sh/v3/syntax"
)

// associativeCommands are the declaration commands whose -A makes a name
// an associative array. export, integer and float reject -A at run time
// ("bad option: -A", zsh 5.9.2), so they are absent.
var associativeCommands = map[string]bool{
	"typeset":  true,
	"local":    true,
	"declare":  true,
	"readonly": true,
	"private":  true,
}

// precommandModifiers may stand before a declaration command without
// changing what it declares. command is not one: it runs only external
// commands, so "command typeset" is "command not found".
var precommandModifiers = map[string]bool{
	"builtin":   true,
	"noglob":    true,
	"nocorrect": true,
}

// associativeTools are the other builtins that can create an associative
// array, each measured under zsh 5.9.2: zparseopts -A, zstat -H (and its
// stat alias) from zsh/stat, ztie from zsh/db/gdbm, and eval.
var associativeTools = map[string]bool{
	"zparseopts": true,
	"zstat":      true,
	"stat":       true,
	"ztie":       true,
	"eval":       true,
}

// specialAssociative lists the special parameters that are associative
// arrays in every shell that loads their module, measured under zsh 5.9.2
// with zsh -f after loading zsh/parameter, zsh/terminfo, zsh/termcap,
// zsh/mapfile, zsh/langinfo and zsh/system (${parameters[name]} reports
// association-...-special). zsh/parameter autoloads, so a file can use
// most of them without declaring anything.
var specialAssociative = map[string]bool{
	"aliases": true, "builtins": true, "commands": true,
	"dis_aliases": true, "dis_builtins": true, "dis_functions": true,
	"dis_functions_source": true, "dis_galiases": true, "dis_saliases": true,
	"functions": true, "functions_source": true, "galiases": true,
	"history": true, "jobdirs": true, "jobstates": true, "jobtexts": true,
	"langinfo": true, "mapfile": true, "modules": true, "nameddirs": true,
	"options": true, "parameters": true, "saliases": true, "sysparams": true,
	"termcap": true, "terminfo": true, "userdirs": true, "usergroups": true,
	"widgets": true,
}

// declArg is one argument of a declaration command: a name the parser
// bound as an assignment, or a word read as text. readable is false when
// the word holds anything but literal text (an expansion, a substitution,
// an ANSI-C string), so its value is unknown before run time.
type declArg struct {
	name     string
	text     string
	readable bool
}

// noteAssociative records which names a declaration makes associative and
// returns the attribute in force at each argument. canAssoc says whether
// the command accepts -A at all.
//
// Options apply to every name after them; Zsh rejects an option placed
// after a name ("not valid in this context"), so the scan need not track
// where options end. Whenever a word cannot be read, the answer errs
// toward associative: an unreadable word may itself be -A, so later names
// are marked, and under -A it may be any name, so every name is.
func (m *Map) noteAssociative(canAssoc bool, args []declArg) []bool {
	assoc := make([]bool, len(args))
	isAssoc := false
	for i, a := range args {
		assoc[i] = isAssoc
		switch {
		case a.name != "":
			if isAssoc {
				m.markAssociative(a.name)
			}
		case !a.readable:
			if isAssoc {
				m.anyAssociative = true
			}
			if canAssoc {
				isAssoc = true
			}
		case strings.HasPrefix(a.text, "-"):
			if canAssoc && strings.ContainsRune(a.text, 'A') {
				isAssoc = true
			}
		case strings.HasPrefix(a.text, "+"):
			if strings.ContainsRune(a.text, 'A') {
				isAssoc = false
			}
		case isIdentifier(a.text):
			// A name the parser did not bind because it was escaped or
			// quoted (typeset -A \v).
			if isAssoc {
				m.markAssociative(a.text)
			}
		case isDigits(a.text):
			// An option argument (typeset -L 5 v).
		default:
			// A word that expands to names Zsh decides later, such as a
			// brace expansion (typeset -A {v,w}).
			if isAssoc {
				m.anyAssociative = true
			}
		}
	}
	return assoc
}

func (m *Map) markAssociative(name string) {
	if m.associative == nil {
		m.associative = make(map[string]bool)
	}
	m.associative[name] = true
}

// noteAssociativeCall handles a command, reached by commandCall, that can
// make a name associative.
func (m *Map) noteAssociativeCall(cmd string, args []*syntax.Word) {
	switch cmd {
	case "eval":
		m.noteEval(args)
	case "zparseopts":
		// zparseopts -A NAME (also -ANAME) stores the options in an
		// associative array. Its own options end at the first word that
		// is not one, or at - / --.
		m.noteOptionName(args, 'A', true)
	case "zstat", "stat":
		// zsh/stat: -H NAME (also -HNAME, or bundled as -nH NAME) fills an
		// associative array.
		m.noteOptionName(args, 'H', false)
	case "ztie":
		// zsh/db/gdbm ties its final argument to an associative array.
		m.noteLastName(args)
	default:
		decl := make([]declArg, len(args))
		for i, word := range args {
			decl[i].text, decl[i].readable = wordText(word)
			if name, _, found := strings.Cut(decl[i].text, "="); found && decl[i].readable && isIdentifier(name) {
				decl[i] = declArg{name: name}
			}
		}
		m.noteAssociative(associativeCommands[cmd], decl)
	}
}

// noteOptionName marks the name that follows letter in a command's
// options, either as the rest of the same word (-ANAME) or as the next
// word (-A NAME, -nA NAME). An unreadable option word may hold the letter,
// and an unreadable name may be any name, so either makes every name a
// possible associative array. stopAtOperand ends the scan at the first
// word that is not an option.
func (m *Map) noteOptionName(args []*syntax.Word, letter byte, stopAtOperand bool) {
	for i := 0; i < len(args); i++ {
		text, ok := wordText(args[i])
		if !ok {
			m.anyAssociative = true
			return
		}
		if text == "-" || text == "--" {
			return
		}
		if len(text) < 2 || (text[0] != '-' && text[0] != '+') {
			if stopAtOperand {
				return
			}
			continue
		}
		at := strings.IndexByte(text[1:], letter)
		if at < 0 || text[0] != '-' {
			continue
		}
		if rest := text[at+2:]; rest != "" {
			m.markName(rest, true)
			continue
		}
		if i+1 >= len(args) {
			return
		}
		i++
		name, ok := wordText(args[i])
		m.markName(name, ok)
	}
}

// noteLastName marks a command's final argument.
func (m *Map) noteLastName(args []*syntax.Word) {
	if len(args) == 0 {
		return
	}
	name, ok := wordText(args[len(args)-1])
	m.markName(name, ok)
}

// markName marks one name, or every name when it cannot be read.
func (m *Map) markName(name string, readable bool) {
	if readable && isIdentifier(name) {
		m.markAssociative(name)
		return
	}
	m.anyAssociative = true
}

// noteEval indexes the source eval runs. A body that is known before run
// time is parsed and indexed for the associative attribute only; one that
// is not, or does not parse, may declare any name.
func (m *Map) noteEval(args []*syntax.Word) {
	if len(args) == 0 {
		return
	}
	parts := make([]string, 0, len(args))
	for _, word := range args {
		text, ok := wordText(word)
		if !ok {
			m.anyAssociative = true
			return
		}
		parts = append(parts, text)
	}
	file, err := parse.Parse(strings.NewReader(strings.Join(parts, " ")+"\n"), "eval")
	if err != nil {
		m.anyAssociative = true
		return
	}
	inner := NewMap()
	inner.Index(file.AST())
	if inner.anyAssociative {
		m.anyAssociative = true
	}
	for name := range inner.associative {
		m.markAssociative(name)
	}
}

// noteAssociativeFlags visits the parameter expansions in a declaration's
// values, which the index walk does not descend into.
func (m *Map) noteAssociativeFlags(node syntax.Node) {
	syntax.Walk(node, func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok {
			m.noteAssociativeFlag(pe)
		}
		return true
	})
}

// noteAssociativeFlag marks the name an ${(AA)name::=...} or
// ${(AA)=name::=...} expansion assigns as an associative array. Any other
// assignment flag leaves the name alone.
func (m *Map) noteAssociativeFlag(pe *syntax.ParamExp) {
	if pe == nil || pe.Flags == nil || pe.Exp == nil || pe.Exp.Op != syntax.AssignUnsetOrNull {
		return
	}
	if strings.Count(pe.Flags.Value, "A") < 2 {
		return
	}
	if pe.Param == nil || pe.Param.Value == "" {
		m.anyAssociative = true
		return
	}
	m.markName(pe.Param.Value, true)
}

// commandCall returns the command a call runs, after quote removal and
// any precommand modifier, when it is one that can make a name
// associative, and the words after it.
func commandCall(words []*syntax.Word) (string, []*syntax.Word) {
	for i, word := range words {
		text, ok := wordText(word)
		if !ok {
			return "", nil
		}
		if precommandModifiers[text] {
			continue
		}
		if associativeCommands[text] || associativeTools[text] {
			return text, words[i+1:]
		}
		return "", nil
	}
	return "", nil
}

// wordText returns the text a word stands for after quote removal, and
// false when it holds anything that is only known at run time.
func wordText(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(unescape(p.Value))
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			// A backslash stays in the text, so an escaped name is not an
			// identifier and is treated as an unknown name.
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

// unescape removes the backslashes of an unquoted literal, keeping the
// character each one quotes.
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
