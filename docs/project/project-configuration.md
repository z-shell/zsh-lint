# Project configuration

The `zsh-lint` project configuration supplies explicit project and source
metadata to rules that need more context than one syntax tree can provide. It
is opt-in. Version 1 performs no automatic configuration discovery.

Pass the configuration path on every configured invocation:

```sh
zsh-lint --config zsh-lint.json plugin.zsh functions/example-run
zsh-lint --format=json --config zsh-lint.json plugin.zsh
```

Invocations without `--config` preserve the unconfigured behavior and default
rule set.

A valid schema version 1 configuration activates the independent
`z-shell/project@1` rule profile. Configuration schema versions describe the
accepted metadata document; rule-profile versions describe a complete,
deterministic rule set. They are deliberately separate so a schema correction
does not silently change rule membership, and a rule-profile revision does not
silently change configuration syntax.

The configured profile contains correctness rules, the organization-approved
`style/prefer-double-brackets` advisory style rule, the existing plugin
lifecycle rules, `plugin/function-namespace`, and evidence-gated advisory
performance rules. The legacy `style/backquotes` and `style/function-decl`
rules remain available only in unconfigured compatibility mode because no
organization policy currently adopts those preferences. Rules that need project
context use explicit `project.kind`, source `profile`, and source `role`
metadata. Paths cannot override configured metadata. The legacy path heuristics
remain available only on invocations without `--config`.

In project profile version 1:

- `plugin/function-scoped-options` applies to plugin and Zi annex
  `autoload-function` sources, including the `completion` role;
- `plugin/zero-handling`, `plugin/unload-function`, and
  `plugin/fpath-hygiene` apply to plugin and Zi annex `sourced-library`
  sources; and
- `plugin/function-namespace` retains its documented plugin and Zi annex
  sourced-library and autoload-function boundary.

Configured invocations analyze their complete explicit input list as one
project. The `plugin/project-unload-lifecycle` validator can therefore match a
persistent registration in an entrypoint to an exact `*_plugin_unload`
definition in another configured source. Files omitted from the command are
intentionally outside that proof boundary.

The `performance/repeated-external-command` rule applies only to configured
`autoload-function` sources with the `completion` role. It reports Info
findings for a conservative allowlist of known external commands inside loops.
Its cost model is N process creations for N iterations; remediation still
requires workload measurement when the command result varies per iteration.

Theme projects do not inherit plugin lifecycle contracts in this profile. A
future theme contract requires an explicit profile decision.

## Version 1 format

```json
{
  "version": 1,
  "project": {
    "kind": "plugin",
    "minimum_zsh": "5.8",
    "function_namespaces": ["example"]
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

- `version` must be `1`.
- `project.kind` must be one of `plugin`, `theme`, `zi-annex`, `module`,
  `library`, `tool`, or `application`.
- `project.minimum_zsh` has two or three numeric components, such as `5.8` or
  `5.9.1`. Components must not have leading zeroes.
- `project.function_namespaces` is an explicit list of namespaces accepted by
  rules that evaluate function names. Each value is matched as a literal name
  prefix. Namespaces are never inferred from a repository or directory name.
  The list may be empty.

### Source fields

Each `sources` entry maps a configuration-relative root to an execution
profile. At least one source entry is required.

`profile` must be one of:

- `standalone-executable`
- `startup-file`
- `sourced-library`
- `autoload-function`
- `test-fixture`

The optional `role` field supports `completion` in version 1. That role requires
the `autoload-function` profile.

Source roots use forward slashes, must be clean relative paths, and must not
escape the directory containing the configuration. Duplicate roots are
invalid. When roots overlap, the longest matching root wins. For example,
`functions/example-run` resolves to `functions` rather than `.` in the example
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
