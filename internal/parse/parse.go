// Package parse wraps the mvdan.cc/sh parser as the analyzer's front end.
//
// The front end uses mvdan/sh's Zsh dialect (LangZsh, available since
// v3.13.x), which on the documented survey corpus parses roughly twice as
// many real Z-Shell files as the Bash variant the reboot started with
// (issues #11, #53). Isolating the front end here lets it be swapped without
// touching callers. Remaining Zsh gaps are tracked as corpus fixtures. Narrow
// compatibility adapters may live at this boundary only when native Zsh and
// the released manual prove the syntax valid, the adapter is gated to one
// parser failure, source positions remain unchanged, and original AST content
// is restored before callers receive the file. See issue #112 and
// docs/project/parser-gap-workflow.md.
package parse

import (
	"bytes"
	"io"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// File is the parsed source produced by the front end.
type File struct {
	tree  *syntax.File
	lines []string
}

// AST returns the underlying mvdan.cc/sh syntax tree.
func (f *File) AST() *syntax.File {
	return f.tree
}

// Lines returns the raw source split into lines (1-based line N is
// Lines()[N-1]); each line excludes its terminating newline, and a final
// newline does not produce a phantom empty line. Suppression-scope
// classification needs real line content: a comment alone on a line inside
// a multi-line construct shares the construct's AST span, so span math
// alone cannot tell trailing from preceding directives.
func (f *File) Lines() []string {
	return f.lines
}

func parseTree(src []byte, name string) (*syntax.File, error) {
	parser := syntax.NewParser(
		syntax.KeepComments(true),
		syntax.Variant(syntax.LangZsh),
	)
	return parser.Parse(bytes.NewReader(src), name)
}

// Parse parses a single Zsh source read from r, using name in error
// messages. It returns the parsed source or a parse error.
func Parse(r io.Reader, name string) (*File, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	tree, err := parseTree(src, name)
	if err != nil {
		tree, err = parseNestedConditionalAlternation(src, name, err)
		if err != nil {
			tree, err = parseAlternateIfBrace(src, name, err)
			if err != nil {
				tree, err = parseAssociativeSubscript(src, name, err)
				if err != nil {
					tree, err = parseRCExpandCaret(src, name, err)
					if err != nil {
						tree, err = parseReverseSubscript(src, name, err)
						if err != nil {
							tree, err = parseParamGlobToggle(src, name, err)
							if err != nil {
								tree, err = parseFdVarRedirect(src, name, err)
								if err != nil {
									tree, err = parseMultiNameFor(src, name, err)
									if err != nil {
										tree, err = parseTryAlways(src, name, err)
										if err != nil {
											tree, err = parseGroupedCasePattern(src, name, err)
											if err != nil {
												return nil, err
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if err := validateConditionalPatterns(src, name); err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(src), "\n")
	return &File{tree: tree, lines: strings.Split(text, "\n")}, nil
}
