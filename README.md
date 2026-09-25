<!-- markdownlint-disable MD041 -->

<div align="center"> <a href="https://github.com/z-shell/zsh-lint">
    <img
      src="https://raw.githubusercontent.com/z-shell/.github/main/profile/img/logo.svg"
      alt="Zsh Lint logo"
      width="72"
      height="72"
    />
</a>

<h1>Zsh Lint</h1> <p>Standalone Go-based semantic analyzer and static linter for Zsh scripts.</p> <p>
    <a href="https://github.com/z-shell/zsh-lint/actions/workflows/go-ci.yml">
      <img
        src="https://github.com/z-shell/zsh-lint/actions/workflows/go-ci.yml/badge.svg?branch=main"
        alt="Go CI status"
      />
    </a>
    <a href="https://github.com/z-shell/zsh-lint/releases">
      <img
        src="https://img.shields.io/github/v/release/z-shell/zsh-lint?display_name=tag&sort=semver"
        alt="Latest release"
      />
    </a>
    <a href="https://go.dev/">
      <img
        src="https://img.shields.io/badge/go-1.26-00add8?logo=go&logoColor=white"
        alt="Go version"
      />
    </a>
    <a href="LICENSE">
      <img
        src="https://img.shields.io/github/license/z-shell/zsh-lint"
        alt="License: GPL-3.0"
      />
    </a>
    <a href="https://wiki.zshell.dev/community/zsh_plugin_standard">
      <img
        src="https://img.shields.io/badge/Zsh_Plugin_Standard-v2_rules-blue"
        alt="Enforces Zsh Plugin Standard v2 rules"
      />
    </a>
</p> </div>

`zsh-lint` is a standalone, Go-based semantic analyzer for Zsh.
It parses shell source code into an abstract syntax tree using `mvdan/sh`, evaluates static analysis rules, enforces [Zsh Plugin Standard v2](https://wiki.zshell.dev/community/zsh_plugin_standard), and reports compiler-style diagnostics or structured JSON.

> [!IMPORTANT]
> Canonical documentation lives on the Z-Shell Wiki:
> **[wiki.zshell.dev/community/zsh_lint](https://wiki.zshell.dev/community/zsh_lint)**.
> Consult the wiki for end-user guides, file selection strategies, suppression syntax, CI workflows, and the published rule reference.

## Features

- **Semantic static analysis:** Evaluates syntax trees for unquoted variables, backquote command substitutions, special parameter shadowing, unsafe `eval` calls, and style issues without executing scripts.
- **Zsh Plugin Standard v2 enforcement:** Every run checks unload functions, function-scoped options, `$0` handling, and `fpath` hygiene.
  Projects with a validated `zsh-lint.json` additionally get the `z-shell/project@2` profile: function and parameter namespaces, the shared `Plugins` registry, load-only helpers, and repeated external commands.
- **Automatic project discovery:** Discovers `zsh-lint.json` configuration files up the directory hierarchy to contextualize standalone scripts, plugin entrypoints, autoloaded functions, and completions.
- **Inline suppression:** Silences one intentional finding at a time with `# zsh-lint disable=<rule-id> -- reason`; suppressions must name a rule and are audited through `meta/*` diagnostics.
- **Greppable and JSON diagnostics:** Outputs standard `file:line:col: [rule] message` diagnostics for terminal and editor workflows, or structured JSON for automated CI checks.
- **Corpus survey tooling:** Includes the companion `zsh-lint-survey` CLI to survey parser front-end coverage across large Zsh codebases without running lint rules.

## Requirements

- **Go 1.26** or newer (when compiling from source or installing via `go install`).

> [!NOTE]
> `zsh-lint` neither sources nor executes the files it analyzes, and it does not call `zsh`. A native syntax check such as `zsh -f -n -- file.zsh` is a separate, recommended step.

## Installation

### Go install

Build and install the latest tagged release:

```bash
go install github.com/z-shell/zsh-lint/cmd/zsh-lint@latest
```

To install the parser survey tool:

```bash
go install github.com/z-shell/zsh-lint/cmd/zsh-lint-survey@latest
```

Ensure `$(go env GOPATH)/bin` is included in your `PATH`.

### Build from source

```bash
git clone https://github.com/z-shell/zsh-lint.git
cd zsh-lint
go build ./cmd/zsh-lint
```

### Zi plugin manager

Let [Zi](https://github.com/z-shell/zi) clone the repository, build the binary with the local Go toolchain, and put it on `PATH`:

```zsh
zi as"program" nocompletions atclone"go build ./cmd/zsh-lint" atpull"%atclone" pick"zsh-lint" for z-shell/zsh-lint
```

## Usage

Pass one or more file paths to `zsh-lint`:

```bash
zsh-lint script.zsh plugin.plugin.zsh
```

### Command-line options

| Option          | Type    | Description                                                        |
| :-------------- | :------ | :----------------------------------------------------------------- |
| `--config PATH` | string  | Explicit project configuration path; disables discovery.           |
| `--no-config`   | boolean | Disable automatic `zsh-lint.json` discovery and use default rules. |
| `--format=json` | string  | Emit machine-readable diagnostics in JSON format.                  |

`--config` and `--no-config` are mutually exclusive.

### Diagnostic output

Standard terminal output format:

```text
script.zsh:2:8: [quoting/unquoted-var] Variable expansion should be double-quoted
plugin.plugin.zsh:1:1: [plugin/unload-function] Plugin registers persistent hooks or widgets but defines no '<name>_plugin_unload' function
```

Findings at `error` or `warning` severity fail the run; `info` and `hint` findings are reported but do not change the exit code.
The [rule reference](https://wiki.zshell.dev/community/zsh_lint/zsh-lint-rule-reference) lists every rule with its severity.

> [!NOTE]
> The CLI analyzes each explicitly provided file path as Zsh source. It does not recurse into directories or filter files by shebang.

## Project configuration

Add a `zsh-lint.json` file at the root of a repository to provide project context for multi-file plugins and tools.
`zsh-lint` walks upward from each input file to find the nearest configuration automatically.

```json
{
  "version": 2,
  "project": {
    "kind": "plugin",
    "minimum_zsh": "5.8",
    "identifier": "example"
  },
  "sources": [
    { "root": "example.plugin.zsh", "profile": "sourced-library" },
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

A validated version 2 configuration selects the `z-shell/project@2` rule profile automatically; there is no profile key to set.
This file is [`examples/plugin/zsh-lint.json`](examples/plugin/zsh-lint.json), and [`examples/standalone`](examples/standalone) shows the `application` kind.
Schema fields and source profiles are described in the [Project Configuration Guide](https://wiki.zshell.dev/community/zsh_lint/zsh-lint-project-configuration).

<details> <summary><strong>Advanced: Parser survey utility (zsh-lint-survey)</strong></summary>

`zsh-lint-survey` executes the parser front end across target files and reports parser gaps without evaluating static analysis rules.
It is used for corpus validation and upstream syntax compatibility tracking.

```bash
zsh-lint-survey path/to/*.zsh
zsh-lint-survey -trace-parses path/to/file.zsh   # also report parse count and adapter depth on stderr
zsh-lint-survey -compare ./base/zsh-lint-survey -native path/to/*.zsh   # verdict changes against a base build
```

</details>

<details> <summary><strong>Advanced: JSON output contract</strong></summary>

`--format=json` writes one versioned envelope to stdout.
Parser failures share it under the reserved rule `parse/error`, and an unpositioned diagnostic omits `range`:

```json
{
  "version": 1,
  "diagnostics": [
    {
      "rule": "quoting/unquoted-var",
      "severity": "warning",
      "message": "Variable expansion should be double-quoted",
      "file": "script.zsh",
      "range": {
        "start": { "line": 2, "column": 8, "offset": 15 },
        "end": { "line": 2, "column": 10, "offset": 17 }
      }
    }
  ],
  "summary": {
    "files": 1,
    "diagnostics": 1,
    "errors": 0,
    "warnings": 1,
    "infos": 0,
    "hints": 0
  }
}
```

The full contract, including sort order and versioning rules, is [`docs/project/output-contract.md`](docs/project/output-contract.md).

</details>

<details> <summary><strong>Exit code conventions</strong></summary>

| Exit code | Description                                                                                                                                                                    |
| :-------: | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|    `0`    | Every input parsed, and no finding at `error` or `warning` severity; `info` and `hint` findings may still be printed.                                                          |
|    `1`    | At least one parser failure, unreadable input file, per-file configuration discovery failure, or finding at `error` or `warning` severity.                                     |
|    `2`    | Invocation error: unknown or repeated flags, no input paths, `--config` with `--no-config`, or an explicit `--config` file that fails to load or does not cover an input path. |

</details>

## Verification

Run the test suite and static analysis from the repository root:

```bash
go build ./... && go vet ./... && go test ./...
golangci-lint run ./...
```

To regenerate API and code-derived reference documentation:

```bash
go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./cmd/zsh-lint-survey ./internal/survey ./internal/rules
```

Contributor workflows and local testing practices are documented in [`docs/README.md`](docs/README.md).
Report parser gaps and propose rules through the [issue forms](https://github.com/z-shell/zsh-lint/issues/new/choose); anything else can be a blank issue.
Pull requests link their owning issue or carry the `meta:no-issue` label.

## Documentation and ecosystem links

- [Z-Shell Wiki: Zsh Lint Documentation](https://wiki.zshell.dev/community/zsh_lint)
- [Zsh Plugin Standard v2](https://wiki.zshell.dev/community/zsh_plugin_standard)
- [Official Zsh Manual](https://zsh.sourceforge.io/Doc/)
- [Zi Plugin Manager](https://github.com/z-shell/zi)
- [Issue Tracker](https://github.com/z-shell/zsh-lint/issues)

## Release model

Contributions integrate on `main` via squash merge after review.
Annotated `vX.Y.Z` tags mark official releases and trigger automated publication workflows.

## Contributing and license

Contributions follow the [Z-Shell Organization Guidelines](https://github.com/z-shell/.github).
Distributed under the terms of the GNU General Public License v3.0.
See [LICENSE](LICENSE) for details.

---

<div align="center"> <p>Developed with ❤️ by the <a href="https://github.com/z-shell">Z-Shell Community</a>.</p> </div>
