// Package parse wraps the mvdan.cc/sh parser as the analyzer's front end.
//
// mvdan/sh has no dedicated Zsh dialect, so the reboot uses the Bash variant as
// the closest available grammar and records Zsh-specific gaps from real code
// (see issues #8, #11–#16). Isolating the front end here lets it be swapped
// later (e.g. tree-sitter-zsh, issue #17) without touching callers.
package parse

import (
	"io"

	"mvdan.cc/sh/v3/syntax"
)

// File is the parsed syntax tree produced by the front end.
type File = syntax.File

// Parse parses a single Zsh/Bash source read from r, using name in error
// messages. It returns the syntax tree or the first parse error.
func Parse(r io.Reader, name string) (*File, error) {
	parser := syntax.NewParser(
		syntax.KeepComments(true),
		syntax.Variant(syntax.LangBash),
	)
	return parser.Parse(r, name)
}
