package rules

import (
	"github.com/z-shell/zsh-lint/internal/analyzer"
)

// Default returns the default set of static analysis rules.
func Default() []analyzer.Rule {
	return []analyzer.Rule{
		UnquotedVar{},
		Backquotes{},
		PreferDoubleBrackets{},
	}
}
