package rules

import (
	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

// Default returns the default set of static analysis rules.
func Default() []analyzer.Rule {
	return []analyzer.Rule{
		UnquotedVar{},
		Backquotes{},
		PreferDoubleBrackets{},
		EvalUsage{},
		FuncDeclStyle{},
		FunctionScopedOptions{},
		SpecialParamShadow{},
		ZeroHandling{},
		UnloadFunction{},
		FpathHygiene{},
	}
}

// ProjectProfile returns the opt-in rules associated with one validated
// project-configuration version. These rules are not part of Default.
func ProjectProfile(version int) []analyzer.Rule {
	if version != projectconfig.CurrentVersion {
		return nil
	}
	return []analyzer.Rule{FunctionNamespace{}}
}
