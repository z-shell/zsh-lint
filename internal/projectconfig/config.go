// Package projectconfig loads and resolves explicit zsh-lint project metadata.
package projectconfig

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// CurrentVersion is the only configuration schema version understood by this
// release.
const CurrentVersion = 2

// ProjectKind identifies the kind of Z-Shell project being analyzed.
type ProjectKind string

const (
	KindPlugin      ProjectKind = "plugin"
	KindTheme       ProjectKind = "theme"
	KindZiAnnex     ProjectKind = "zi-annex"
	KindModule      ProjectKind = "module"
	KindLibrary     ProjectKind = "library"
	KindTool        ProjectKind = "tool"
	KindApplication ProjectKind = "application"
)

// Profile identifies how a source file executes in Zsh.
type Profile string

const (
	ProfileStandaloneExecutable Profile = "standalone-executable"
	ProfileStartupFile          Profile = "startup-file"
	ProfileSourcedLibrary       Profile = "sourced-library"
	ProfileAutoloadFunction     Profile = "autoload-function"
	ProfileTestFixture          Profile = "test-fixture"
)

// SourceRole refines a source profile when a rule needs a narrower contract.
// The empty role is ordinary source; completion is the only version 2 role.
type SourceRole string

const RoleCompletion SourceRole = "completion"

// Config is the validated version 2 project configuration.
type Config struct {
	Version int
	Project Project
	Sources []Source

	root string
}

// Project contains project-wide metadata shared by every configured source.
type Project struct {
	Kind       ProjectKind
	MinimumZsh string
	Identifier string
}

// Source maps a project-relative root to one execution profile and optional
// role. More-specific roots take precedence during resolution.
type Source struct {
	Root    string
	Profile Profile
	Role    SourceRole
}

// SourceContext is immutable-by-convention metadata supplied to analyzer
// rules for one resolved input file.
type SourceContext struct {
	ConfigVersion     int
	ProjectKind       ProjectKind
	MinimumZsh        string
	ProjectIdentifier string
	Profile           Profile
	Role              SourceRole
	ConfigRoot        string
	SourceRoot        string
}

// Configured reports whether this context came from a configured source.
func (c SourceContext) Configured() bool { return c.Profile != "" }

// Clone returns an independent copy of the source context.
func (c SourceContext) Clone() SourceContext { return c }

func (c *Config) validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("version: unsupported schema version %d (want %d)", c.Version, CurrentVersion)
	}
	if !validProjectKind(c.Project.Kind) {
		return fmt.Errorf("project.kind: unknown project kind %q", c.Project.Kind)
	}
	if err := validateZshVersion(c.Project.MinimumZsh); err != nil {
		return fmt.Errorf("project.minimum_zsh: %w", err)
	}

	if err := validateProjectIdentifier(c.Project.Identifier); err != nil {
		return fmt.Errorf("project.identifier: %w", err)
	}

	if len(c.Sources) == 0 {
		return fmt.Errorf("sources: must contain at least one source root")
	}
	seenRoots := make(map[string]bool, len(c.Sources))
	for index, source := range c.Sources {
		field := fmt.Sprintf("sources[%d]", index)
		if err := validateSourceRoot(source.Root); err != nil {
			return fmt.Errorf("%s.root: %w", field, err)
		}
		if seenRoots[source.Root] {
			return fmt.Errorf("%s.root: duplicate source root %q", field, source.Root)
		}
		seenRoots[source.Root] = true
		if !validProfile(source.Profile) {
			return fmt.Errorf("%s.profile: unknown execution profile %q", field, source.Profile)
		}
		if source.Role != "" && source.Role != RoleCompletion {
			return fmt.Errorf("%s.role: unknown source role %q", field, source.Role)
		}
		if source.Role == RoleCompletion && source.Profile != ProfileAutoloadFunction {
			return fmt.Errorf("%s.role: completion requires profile %q", field, ProfileAutoloadFunction)
		}
		if c.Project.Kind == KindPlugin || c.Project.Kind == KindZiAnnex {
			if err := validatePluginSourceMapping(source); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
	}
	return nil
}

func validateProjectIdentifier(identifier string) error {
	if identifier == "" {
		return fmt.Errorf("must not be empty")
	}
	for index, char := range []byte(identifier) {
		letter := char >= 'a' && char <= 'z'
		digit := char >= '0' && char <= '9'
		if !letter && !digit && char != '-' {
			return fmt.Errorf("must contain only lowercase portable ASCII letters, digits, and hyphens")
		}
		if (index == 0 || index == len(identifier)-1) && !letter {
			return fmt.Errorf("must begin and end with a lowercase ASCII letter")
		}
	}
	return nil
}

// ShellPrefix derives the identifier form used by persistent Zsh function and
// parameter names. Hyphens remain available in repository and zstyle names,
// while underscores keep the shell-visible prefix portable and easy to type.
func ShellPrefix(identifier string) string {
	return strings.ReplaceAll(identifier, "-", "_")
}

func validatePluginSourceMapping(source Source) error {
	switch source.Root {
	case "lib":
		if source.Profile != ProfileSourcedLibrary || source.Role != "" {
			return fmt.Errorf("root %q must use profile %q with no role", source.Root, ProfileSourcedLibrary)
		}
	case "functions":
		if source.Profile != ProfileAutoloadFunction || source.Role != "" {
			return fmt.Errorf("root %q must use profile %q with no role", source.Root, ProfileAutoloadFunction)
		}
	case "completions":
		if source.Profile != ProfileAutoloadFunction || source.Role != RoleCompletion {
			return fmt.Errorf("root %q must use profile %q and role %q", source.Root, ProfileAutoloadFunction, RoleCompletion)
		}
	default:
		if strings.HasSuffix(source.Root, ".plugin.zsh") && source.Profile != ProfileSourcedLibrary {
			return fmt.Errorf("plugin entrypoint %q must use profile %q", source.Root, ProfileSourcedLibrary)
		}
	}
	return nil
}

func validProjectKind(kind ProjectKind) bool {
	switch kind {
	case KindPlugin, KindTheme, KindZiAnnex, KindModule, KindLibrary, KindTool, KindApplication:
		return true
	default:
		return false
	}
}

func validProfile(profile Profile) bool {
	switch profile {
	case ProfileStandaloneExecutable, ProfileStartupFile, ProfileSourcedLibrary,
		ProfileAutoloadFunction, ProfileTestFixture:
		return true
	default:
		return false
	}
}

func validateZshVersion(version string) error {
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("must contain two or three numeric components")
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("must contain two or three numeric components")
		}
		if len(part) > 1 && part[0] == '0' {
			return fmt.Errorf("component %q has a leading zero", part)
		}
		if _, err := strconv.ParseUint(part, 10, 31); err != nil {
			return fmt.Errorf("component %q is not a non-negative integer", part)
		}
	}
	return nil
}

func validateSourceRoot(root string) error {
	if root == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.ContainsRune(root, '\x00') {
		return fmt.Errorf("must not contain NUL")
	}
	if strings.ContainsRune(root, '\\') {
		return fmt.Errorf("must use forward slashes")
	}
	if path.IsAbs(root) {
		return fmt.Errorf("must be relative to the configuration directory")
	}
	if len(root) >= 2 && root[1] == ':' &&
		((root[0] >= 'A' && root[0] <= 'Z') || (root[0] >= 'a' && root[0] <= 'z')) {
		return fmt.Errorf("must not contain a Windows drive prefix")
	}
	if cleaned := path.Clean(root); cleaned != root {
		return fmt.Errorf("must be clean (use %q)", cleaned)
	}
	if root == ".." || strings.HasPrefix(root, "../") {
		return fmt.Errorf("must not escape the configuration directory")
	}
	return nil
}
