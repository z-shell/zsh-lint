// Package parse wraps the mvdan.cc/sh parser as the analyzer's front end.
//
// The front end uses mvdan/sh's Zsh dialect (LangZsh, available since
// v3.13.x), which on the documented survey corpus parses roughly twice as
// many real Z-Shell files as the Bash variant the reboot started with
// (issues #11, #53). Remaining Zsh gaps are tracked as corpus fixtures (see
// issues #12, #13, #15 and docs/project/parser-gap-workflow.md). Isolating
// the front end here lets it be swapped without touching callers.
package parse

import (
	"io"

	"mvdan.cc/sh/v3/syntax"
)

// File is the parsed source produced by the front end.
type File struct {
	tree *syntax.File
}

// AST returns the underlying mvdan.cc/sh syntax tree.
func (f *File) AST() *syntax.File {
	return f.tree
}

// Parse parses a single Zsh source read from r, using name in error
// messages. It returns the parsed source or the first parse error.
func Parse(r io.Reader, name string) (*File, error) {
	parser := syntax.NewParser(
		syntax.KeepComments(true),
		syntax.Variant(syntax.LangZsh),
	)
	tree, err := parser.Parse(r, name)
	if err != nil {
		return nil, err
	}
	return &File{tree: tree}, nil
}
