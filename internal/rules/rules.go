package rules

import (
	"fmt"

	"github.com/z-shell/zsh-lint/internal/analyzer"
)

// Profile identifies one complete, versioned configured rule set. Profile
// versions are owned by the rule registry and are independent from project
// configuration schema versions.
type Profile string

const (
	// ProjectProfileV2 enforces the clean portable plugin architecture defined
	// by Zsh Plugin Standard 2.
	ProjectProfileV2 Profile = "z-shell/project@2"

	// CurrentProjectProfile is selected for validated project configurations in
	// this release.
	CurrentProjectProfile = ProjectProfileV2
)

// Default returns the default set of static analysis rules.
func Default() []analyzer.Rule {
	return append(genericRules(), unconfiguredProjectRules()...)
}

// ForProfile returns the complete rule set associated with a configured rule
// profile. Unknown profiles fail explicitly rather than silently disabling
// configured rules.
func ForProfile(profile Profile) ([]analyzer.Rule, error) {
	switch profile {
	case ProjectProfileV2:
		return configuredProjectRules(), nil
	default:
		return nil, fmt.Errorf("unsupported rule profile %q", profile)
	}
}

func genericRules() []analyzer.Rule {
	return []analyzer.Rule{
		UnquotedVar{},
		Backquotes{},
		PreferDoubleBrackets{},
		EvalUsage{},
		FuncDeclStyle{},
		SpecialParamShadow{},
	}
}

func unconfiguredProjectRules() []analyzer.Rule {
	return []analyzer.Rule{
		FunctionScopedOptions{},
		ZeroHandling{},
		UnloadFunction{},
		ProjectUnloadLifecycle{},
		FpathHygiene{},
	}
}

func configuredProjectRules() []analyzer.Rule {
	return []analyzer.Rule{
		UnquotedVar{},
		PreferDoubleBrackets{},
		EvalUsage{},
		SpecialParamShadow{},
		FunctionScopedOptions{},
		ZeroHandling{},
		UnloadFunction{},
		ProjectUnloadLifecycle{},
		FpathHygiene{},
		FunctionNamespace{},
		SharedPluginsRegistry{},
		PersistentParameterNamespace{},
		LoadOnlyHelper{},
		RepeatedExternalCommand{},
	}
}
