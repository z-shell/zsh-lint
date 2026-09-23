// Package parse wraps the mvdan.cc/sh parser as the analyzer's front end.
//
// The front end uses mvdan/sh's Zsh dialect (LangZsh, available since
// v3.13.x, pinned at v3.14.1), which on the documented survey corpus parses roughly twice as
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
	tree                 *syntax.File
	lines                []string
	anonymousInvocations []AnonymousInvocation
	assignAlways         []*syntax.ParamExp
	secondSubscripts     []SecondSubscript
	repeatLoops          []RepeatLoop
	mathFunctionCalls    []MathFunctionCall
}

// AnonymousInvocation pairs an anonymous function declaration with the words
// passed when Zsh invokes it immediately. mvdan/sh (through v3.14.1) has no AST field
// for these words, so the compatibility front end retains them as typed syntax
// nodes with their original source positions.
type AnonymousInvocation struct {
	Function *syntax.FuncDecl
	Words    []*syntax.Word
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

// AnonymousInvocations returns immediate anonymous-function invocations found
// by the Zsh compatibility front end. The returned slice is independent; its
// syntax nodes are shared with the immutable parse result.
func (f *File) AnonymousInvocations() []AnonymousInvocation {
	return append([]AnonymousInvocation(nil), f.anonymousInvocations...)
}

// AssignAlwaysExpansions returns the parameter expansions written with the
// native unconditional assignment operator `${name::=word}` (issue #216).
// mvdan/sh (through v3.14.1) has no operator for it, so the compatibility
// front end parses the expansion as the conditional `:=` form and records the
// owning node here; consumers that distinguish the two must inspect this
// metadata rather than the tree's Exp.Op. The returned slice is independent;
// its nodes are shared with the immutable parse result.
func (f *File) AssignAlwaysExpansions() []*syntax.ParamExp {
	return append([]*syntax.ParamExp(nil), f.assignAlways...)
}

// SecondSubscripts returns the parameter expansions written with more than
// one subscript, `${name[a][b]}` (issue #215), with the subscripts after the
// first. mvdan/sh (through v3.14.1) has one index per expansion, so the
// compatibility front end keeps the first subscript in the tree's Index and
// records the rest here; consumers that need them must inspect this metadata.
// The returned slice is independent; its nodes are shared with the immutable
// parse result.
func (f *File) SecondSubscripts() []SecondSubscript {
	return append([]SecondSubscript(nil), f.secondSubscripts...)
}

// MathFunctionCalls returns every math function call written in an arithmetic
// expression, `$(( sqrt(4) ))`, in source order. mvdan/sh has no call node in
// its arithmetic grammar, so the front end keeps each call's name and
// arguments here with their original source positions.
func (f *File) MathFunctionCalls() []MathFunctionCall {
	return f.mathFunctionCalls
}

// RepeatLoops returns every `repeat count sublist` loop in source order.
// mvdan/sh (through v3.14.1) has no node for the loop, so the compatibility
// front end rewrites each one into a WhileClause positioned at the `repeat`
// word whose single condition statement is the count word; consumers that
// must tell the two loops apart inspect this metadata. The returned slice is
// independent; its nodes are shared with the immutable parse result.
func (f *File) RepeatLoops() []RepeatLoop {
	return append([]RepeatLoop(nil), f.repeatLoops...)
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
	tree, anonymousInvocations, err := parseFull(src, name)
	if err != nil {
		return nil, err
	}
	tree, anonymousInvocations, err = resolveRepeatLoops(src, name, tree, anonymousInvocations)
	if err != nil {
		return nil, err
	}
	if err := resolveNestedArithmetic(src, name, tree); err != nil {
		return nil, err
	}
	if err := validateConditionalPatterns(src, name); err != nil {
		return nil, err
	}
	if err := rejectCarriageReturns(src, name); err != nil {
		return nil, err
	}
	if err := rejectUnsupportedLoopWords(tree, name); err != nil {
		return nil, err
	}
	if err := rejectCloseBraceWords(tree, name); err != nil {
		return nil, err
	}
	repeatLoops, err := bindRepeatLoops(tree, src, name)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(src), "\n")
	return &File{
		tree:                 tree,
		lines:                strings.Split(text, "\n"),
		anonymousInvocations: anonymousInvocations,
		assignAlways:         bindAssignAlwaysExpansions(tree, src),
		secondSubscripts:     bindSecondSubscripts(tree, src),
		repeatLoops:          repeatLoops,
		mathFunctionCalls:    bindMathFunctionCalls(tree, src),
	}, nil
}

// parseFull runs the adapter chain and, when it still fails, the anonymous
// function invocation retry, which is the whole front end short of the
// `repeat` rewrite that reparses through it.
func parseFull(src []byte, name string) (*syntax.File, []AnonymousInvocation, error) {
	tree, err := parseWithAdapters(src, name)
	if err == nil {
		return tree, nil, nil
	}
	return parseAnonymousFunctionArgs(src, name, err)
}
