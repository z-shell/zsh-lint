package parse

import (
	"mvdan.cc/sh/v3/syntax"
)

// ifEmptyBranchErrors are the parser errors an `if` branch beginning with a
// separator produces. `then;` or `else;` on one line is read as an empty
// branch, so the parser then demands `fi` and reports it at the `if`; a `;`
// after a newline or a comment is instead rejected where it stands.
var ifEmptyBranchErrors = []string{
	"`if` statement must end with `fi`",
	"`;` can only immediately follow a statement",
}

// ifBranchKeywords are the reserved words an `if` branch list follows: the
// `then` of `if` and `elif`, and `else`.
var ifBranchKeywords = map[string]bool{"then": true, "else": true}

// parseThenLeadingSeparator adapts an `if` branch that begins with a
// separator directly after `then` or `else` (issue #297):
// `if true; then; print x; fi`. Zsh reads `then list` and `else list` with
// a list of zero or more sublists, so a leading `;` is an empty sublist. The
// parser (mvdan/sh through v3.14.1) takes the `;` as the whole branch and
// demands `fi`, `elif` or `else` next. This is the `do` case of issue #238
// on another construct, so the shared parseLeadingSeparatorWithParser masks
// that `;` and verifies the tree holds an `if` branch whose keyword is the
// site; an `if` that is genuinely unterminated keeps the parser error.
func parseThenLeadingSeparator(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseThenLeadingSeparatorWithParser(src, name, firstErr, parseWithAdapters)
}

func parseThenLeadingSeparatorWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	spec := leadingSeparatorSpec{errors: ifEmptyBranchErrors, keywords: ifBranchKeywords, verify: hasIfBranch}
	return parseLeadingSeparatorWithParser(src, name, firstErr, spec, parse)
}

// hasIfBranch reports whether the tree holds an `if` branch whose keyword
// is at offset keyword: the `then` of an `if` or `elif` clause, or an `else`
// clause, which the parser represents as an IfClause without a `then`.
func hasIfBranch(tree *syntax.File, keyword int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		clause, ok := node.(*syntax.IfClause)
		if !ok {
			return true
		}
		if clause.ThenPos.IsValid() {
			found = int(clause.ThenPos.Offset()) == keyword
		} else {
			found = int(clause.Position.Offset()) == keyword
		}
		return !found
	})
	return found
}
