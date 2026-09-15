<!-- markdownlint-disable MD041 -->

<div align="center">
  <a href="https://github.com/z-shell/zsh-lint">
    <img
      src="https://raw.githubusercontent.com/z-shell/.github/main/profile/img/logo.svg"
      alt="Zsh Lint logo"
      width="72"
      height="72"
    />
  </a>

  <h1>Zsh Lint</h1>
  <p>Standalone Go-based semantic analyzer and static linter for Zsh scripts.</p>
  <p>
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
        src="https://img.shields.io/badge/standard-v2-blue"
        alt="Zsh Plugin Standard v2 compliance"
      />
    </a>
  </p>
</div>

`zsh-lint` is a standalone, Go-based semantic analyzer for Zsh. It parses shell source code into an abstract syntax tree using `mvdan/sh`, evaluates static analysis rules, enforces [Zsh Plugin Standard v2](https://wiki.zshell.dev/community/zsh_plugin_standard), and reports compiler-style diagnostics or structured JSON.

> [!IMPORTANT]
> Canonical documentation lives on the Z-Shell Wiki:
> **[wiki.zshell.dev/community/zsh_lint](https://wiki.zshell.dev/community/zsh_lint)**.
> Consult the wiki for end-user guides, file selection strategies, suppression syntax, CI workflows, and the published rule reference.

## Features

- **Semantic static analysis:** Evaluates syntax trees for unquoted variables, backquote command substitutions, special parameter shadowing, unsafe `eval` calls, and style issues without executing scripts.
- **Zsh Plugin Standard v2 enforcement:** Validates plugin conventions, including isolated function and parameter namespaces, required unload functions, function-scoped options, zero handling, and absence of shared plugin registries.
- **Automatic project discovery:** Discovers `zsh-lint.json` configuration files up the directory hierarchy to contextualize standalone scripts, plugin entrypoints, autoloaded functions, and completions.
- **Greppable and JSON diagnostics:** Outputs standard `file:line:col: [rule] message` diagnostics for terminal and editor workflows, or structured JSON for automated CI checks.
- **Corpus survey tooling:** Includes the companion `zsh-lint-survey` CLI to survey parser front-end coverage across large Zsh codebases without running lint rules.

## Requirements

- **Go 1.26** or newer (when compiling from source or installing via `go install`).
- **Zsh** (optional, recommended for runtime syntax pre-validation via `zsh -n`).

## Installation

### Go install

Install the latest release binary:

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

Run `zsh-lint` as a managed command inside a Zi environment:

```zsh
zi as"command" from"gh" make"go build ./cmd/zsh-lint" sbin"zsh-lint" for z-shell/zsh-lint
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
script.zsh:12:5: [quoting/unquoted-var] Variable expansion should be double-quoted
plugin.plugin.zsh:45:1: [lifecycle/unload-function] Plugin entrypoint missing unload function
```

> [!NOTE]
> The CLI analyzes each explicitly provided file path as Zsh source. It does not recurse into directories or filter files by shebang.

## Project configuration

Add a `zsh-lint.json` file at the root of a repository to provide project context for multi-file plugins and tools. `zsh-lint` walks upward from each input file to find the nearest configuration automatically.

```json
{
  "version": 1,
  "kind": "zsh-plugin",
  "name": "example-plugin",
  "rules": {
    "profile": "z-shell/project@2"
  },
  "sources": [
    {
      "pattern": "*.plugin.zsh",
      "role": "plugin-entrypoint"
    },
    {
      "pattern": "functions/*",
      "role": "autoloaded-function"
    }
  ]
}
```

Detailed schema definitions and source profiles are described in the [Project Configuration Guide](https://wiki.zshell.dev/community/zsh_lint/zsh-lint-project-configuration).

<details>
<summary><strong>Advanced: Parser survey utility (zsh-lint-survey)</strong></summary>

`zsh-lint-survey` executes the parser front end across target files and reports parser gaps without evaluating static analysis rules. It is used for corpus validation and upstream syntax compatibility tracking.

```bash
zsh-lint-survey path/to/*.zsh
```

</details>

<details>
<summary><strong>Advanced: JSON output contract</strong></summary>

When invoked with `--format=json`, diagnostics are formatted as structured JSON:

```json
{
  "inspected_files": 1,
  "diagnostics": [
    {
      "rule_id": "quoting/unquoted-var",
      "severity": "warning",
      "file": "script.zsh",
      "range": {
        "start": {
          "line": 12,
          "column": 5,
          "offset": 140
        },
        "end": {
          "line": 12,
          "column": 5,
          "offset": 140
        }
      },
      "message": "Variable expansion should be double-quoted"
    }
  ]
}
```

</details>

<details>
<summary><strong>Exit code conventions</strong></summary>

| Exit code | Description                                                                   |
| :-------: | :---------------------------------------------------------------------------- |
|    `0`    | Clean run: all input files parsed without warnings or errors.                 |
|    `1`    | Findings: parser errors or lint warnings were detected.                       |
|    `2`    | Invocation error: invalid flags, missing arguments, or invalid configuration. |

</details>

## Verification

Run the test suite and static analysis from the repository root:

```bash
go build ./... && go vet ./... && go test ./...
```

To regenerate API and code-derived reference documentation:

```bash
go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./cmd/zsh-lint-survey ./internal/survey ./internal/rules
```

Contributor workflows and local testing practices are documented in [`docs/README.md`](docs/README.md). Report parser gaps and propose rules through the [issue forms](https://github.com/z-shell/zsh-lint/issues/new/choose); anything else can be a blank issue. Pull requests link their owning issue or carry the `meta:no-issue` label.

## Documentation and ecosystem links

- [Z-Shell Wiki: Zsh Lint Documentation](https://wiki.zshell.dev/community/zsh_lint)
- [Zsh Plugin Standard v2](https://wiki.zshell.dev/community/zsh_plugin_standard)
- [Official Zsh Manual](https://zsh.sourceforge.io/Doc/)
- [Zi Plugin Manager](https://github.com/z-shell/zi)
- [Issue Tracker](https://github.com/z-shell/zsh-lint/issues)

## Release model

Contributions integrate on `main` via squash merge after review. Annotated `vX.Y.Z` tags mark official releases and trigger automated publication workflows.

## Contributing and license

Contributions follow the [Z-Shell Organization Guidelines](https://github.com/z-shell/.github).
Distributed under the terms of the GNU General Public License v3.0. See [LICENSE](LICENSE) for details.

---

<div align="center">
  <p>Developed with ❤️ by the <a href="https://github.com/z-shell">Z-Shell Community</a>.</p>
</div>
