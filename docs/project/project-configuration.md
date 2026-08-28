# Project configuration

The `zsh-lint` project configuration supplies explicit project and source
metadata to rules that need more context than one syntax tree can provide. It
is opt-in. Version 2 performs no automatic configuration discovery.

Pass the configuration path on every configured invocation:

```sh
zsh-lint --config zsh-lint.json plugin.zsh functions/example_run
zsh-lint --format=json --config zsh-lint.json plugin.zsh
```

Invocations without `--config` preserve the unconfigured behavior and default
rule set.

A valid schema version 2 configuration activates the independent
`z-shell/project@2` rule profile. Configuration schema versions describe the
accepted metadata document; rule-profile versions describe a complete,
deterministic rule set. They are deliberately separate so a schema correction
does not silently change rule membership, and a rule-profile revision does not
silently change configuration syntax.

The configured profile contains correctness rules, the organization-approved
`style/prefer-double-brackets` advisory style rule, the existing plugin
lifecycle rules, Zsh Plugin Standard 2 architecture rules, and evidence-gated
advisory performance rules. The generic `style/backquotes` and
`style/function-decl`
rules remain available only in unconfigured mode because no
organization policy currently adopts those preferences. Rules that need project
context use explicit `project.kind`, source `profile`, and source `role`
metadata. Paths cannot override configured metadata. Path-based applicability
remains available only on generic invocations without `--config` and is not a
Standard 2 compatibility profile.

In project profile version 2:

- `plugin/function-scoped-options` applies to plugin and Zi annex
  `autoload-function` sources, including the `completion` role;
- `plugin/zero-handling`, `plugin/unload-function`, and
  `plugin/fpath-hygiene` apply to plugin and Zi annex `sourced-library`
  sources; and
- `plugin/function-namespace` accepts only the derived public
  `project_name_...` and private `_project_name_...` forms, plus native
  `_command` completion names;
- `plugin/shared-plugins-registry` rejects writes to the ownerless `Plugins`
  parameter;
- `plugin/persistent-parameter-namespace` requires persistent typed state to
  use the private project prefix and directs ordinary configuration to the
  `:project-identifier:config` style context; and
- `plugin/load-only-helper` reports conservatively proven private setup helpers
  that remain defined after initialization.

Configured invocations analyze their complete explicit input list as one
project. The `plugin/project-unload-lifecycle` validator can therefore match a
persistent registration in an entrypoint to an exact `*_plugin_unload`
definition in another configured source. This proves only that an unload
entrypoint is present. Exact ownership-aware restoration is a runtime contract
and must be tested in a clean Zsh process. Files omitted from the command are
intentionally outside that static proof boundary.

The `performance/repeated-external-command` rule applies only to configured
`autoload-function` sources with the `completion` role. It reports Info
findings for a conservative allowlist of known external commands inside loops.
Its cost model is N process creations for N iterations; remediation still
requires workload measurement when the command result varies per iteration.

Theme projects do not inherit plugin lifecycle contracts in this profile. A
future theme contract requires an explicit profile decision.

## Version 2 format

```json
{
  "version": 2,
  "project": {
    "kind": "plugin",
    "minimum_zsh": "5.8",
    "identifier": "example"
  },
  "sources": [
    { "root": ".", "profile": "sourced-library" },
    { "root": "functions", "profile": "autoload-function" },
    {
      "root": "completions",
      "profile": "autoload-function",
      "role": "completion"
    },
    { "root": "tests", "profile": "test-fixture" }
  ]
}
```

All fields shown above are required except `sources[].role`.

### Project fields

- `version` must be `2`.
- `project.kind` must be one of `plugin`, `theme`, `zi-annex`, `module`,
  `library`, `tool`, or `application`.
- `project.minimum_zsh` has two or three numeric components, such as `5.8` or
  `5.9.1`. Components must not have leading zeroes.
- `project.identifier` is the one portable project identifier. It begins and
  ends with a lowercase ASCII letter and otherwise contains lowercase ASCII
  letters, digits, or hyphens. Namespaces are never inferred from a repository
  or directory name. Shell-visible names replace identifier hyphens with
  underscores: `zsh-fancy-completions` derives public prefix
  `zsh_fancy_completions_` and private prefix `_zsh_fancy_completions_`.

### Source fields

Each `sources` entry maps a configuration-relative root to an execution
profile. At least one source entry is required.

`profile` must be one of:

- `standalone-executable`
- `startup-file`
- `sourced-library`
- `autoload-function`
- `test-fixture`

The optional `role` field supports `completion` in version 2. That role requires
the `autoload-function` profile.

For plugin and Zi annex projects, standard directory mappings are validated
when present: `lib` uses `sourced-library`, `functions` uses
`autoload-function`, and `completions` uses `autoload-function` with role
`completion`. A source root ending in `.plugin.zsh` uses `sourced-library`.

Source roots use forward slashes, must be clean relative paths, and must not
escape the directory containing the configuration. Duplicate roots are
invalid. When roots overlap, the longest matching root wins. For example,
`functions/example_run` resolves to `functions` rather than `.` in the example
configuration.

Resolution is lexical. The linter does not execute files or resolve symlinks to
classify an input. Every configured input must remain beneath the configuration
directory and match one source root.

## Validation and errors

Configuration parsing is strict and accepts exactly one UTF-8 JSON document of
at most 1 MiB. Unknown, duplicate, or case-mismatched field names are errors.

The CLI loads the configuration and resolves all input paths before parsing any
source file. A configuration or source-mapping error is written to standard
error, produces no source diagnostics, and exits with status 2. Findings retain
the existing exit-status contract: status 1 when an error or warning is
reported, otherwise status 0.
