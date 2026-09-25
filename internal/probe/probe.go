// Package probe generates probe grids for parser changes (#428): each body,
// a variant of the construct under repair, placed in each context a
// compatibility adapter's source scanner can meet. The grid is judged by
// `zsh-lint-survey -compare <base> -native`, which classifies every file's
// verdict change against native Zsh.
package probe

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Slot marks where a context template takes the body.
const Slot = "@BODY@"

// Context is a Zsh template with exactly one Slot.
type Context struct {
	Name     string
	Template string
}

// Contexts are the places a source scanner has to track to find a construct:
// statement positions inside every compound command, command substitutions
// with and without double quotes and backquotes, and positions after the
// quoting and here-document shapes scanners have misread (#280, #393).
// Every template is valid Zsh when the slot holds a simple command.
var Contexts = []Context{
	{"top", "@BODY@\n"},
	{"function", "f() {\n@BODY@\n}\n"},
	{"function-keyword", "function f {\n@BODY@\n}\n"},
	{"anonymous-function", "() {\n@BODY@\n}\n"},
	{"subshell", "(\n@BODY@\n)\n"},
	{"brace-group", "{\n@BODY@\n}\n"},
	{"if-then", "if true; then\n@BODY@\nfi\n"},
	{"if-else", "if false; then\n  :\nelse\n@BODY@\nfi\n"},
	{"while-do", "while false; do\n@BODY@\ndone\n"},
	{"until-do", "until true; do\n@BODY@\ndone\n"},
	{"for-do", "for x in a; do\n@BODY@\ndone\n"},
	{"repeat-do", "repeat 1 do\n@BODY@\ndone\n"},
	{"case-arm", "case x in\n  (x)\n@BODY@\n  ;;\nesac\n"},
	{"always-block", "{\n  :\n} always {\n@BODY@\n}\n"},
	{"after-and", "true &&\n@BODY@\n"},
	{"after-or", "false ||\n@BODY@\n"},
	{"command-substitution", ": $(\n@BODY@\n)\n"},
	{"quoted-command-substitution", ": \"$(\n@BODY@\n)\"\n"},
	{"backquotes", ": `\n@BODY@\n`\n"},
	{"process-substitution", ": <(\n@BODY@\n)\n"},
	{"after-here-document", "cat <<'EOF'\n$( ) \" ' `\nEOF\n@BODY@\n"},
	{"after-quoted-substitution", ": \"$(print \"it's\")\"\n@BODY@\n"},
	{"after-single-quotes", ": 'a \"b\" $(c) `d`'\n@BODY@\n"},
	{"after-arithmetic", "(( x = 1 << 2 ))\n@BODY@\n"},
	{"after-comment", "# $( \" ' ` <<EOF\n@BODY@\n"},
	{"after-parameter-word", ": ${x:-\"$(print ')')\"}\n@BODY@\n"},
}

// ReadBodies reads bodies separated by lines holding only "---". Leading and
// trailing blank lines of each body are dropped; empty bodies are skipped.
func ReadBodies(r io.Reader) ([]string, error) {
	var bodies []string
	var current []string
	flush := func() {
		body := strings.Trim(strings.Join(current, "\n"), "\n")
		if strings.TrimSpace(body) != "" {
			bodies = append(bodies, body)
		}
		current = current[:0]
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		current = append(current, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return bodies, nil
}

// File is one generated probe: a body placed in a context.
type File struct {
	Name    string
	Content string
}

// Generate places every body in every context. File names are
// "b<body>-<context>.zsh" with the body's 1-based index zero-padded, so the
// files sort by body and a -compare line names both halves.
func Generate(bodies []string, contexts []Context) ([]File, error) {
	width := len(fmt.Sprint(len(bodies)))
	files := make([]File, 0, len(bodies)*len(contexts))
	for i, body := range bodies {
		for _, context := range contexts {
			if strings.Count(context.Template, Slot) != 1 {
				return nil, fmt.Errorf("context %s: template must hold %s exactly once", context.Name, Slot)
			}
			files = append(files, File{
				Name:    fmt.Sprintf("b%0*d-%s.zsh", width, i+1, context.Name),
				Content: strings.Replace(context.Template, Slot, body, 1),
			})
		}
	}
	return files, nil
}

// Write writes the files into dir, which must not exist yet, so a grid never
// mixes with an earlier one.
func Write(dir string, files []File) error {
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file.Name), []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
