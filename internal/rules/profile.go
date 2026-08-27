package rules

import "github.com/z-shell/zsh-lint/internal/projectconfig"

// configuredPluginSource reports whether explicit project metadata selects a
// plugin lifecycle contract for one of the supplied source profiles. Theme
// projects are intentionally excluded until a focused contract is approved.
func configuredPluginSource(source projectconfig.SourceContext, profiles ...projectconfig.Profile) bool {
	if !source.Configured() ||
		(source.ProjectKind != projectconfig.KindPlugin && source.ProjectKind != projectconfig.KindZiAnnex) {
		return false
	}
	for _, profile := range profiles {
		if source.Profile == profile {
			return true
		}
	}
	return false
}
