# Project Guidelines — zsh-lint

This project follows the organization-wide [Z-Shell Organization Guidelines](https://github.com/z-shell/.github/blob/main/AGENTS.md).

## What this is

`zsh-lint` is being rebooted as a **Go-based semantic analyzer for Zsh** (see
[#5](https://github.com/z-shell/zsh-lint/issues/5)). The active product is the
Go code under `cmd/` and `internal/`; the parser front end uses
[`mvdan/sh`](https://github.com/mvdan/sh). It currently operates as a
**parser-survey** tool and implements no lint rules yet.

The original interactive Zi/`.zshrc` plugin lives under `legacy/` and is **not**
part of the active product surface.

## Layout

- `cmd/zsh-lint/` — CLI entry point.
- `internal/parse/` — parser front end (mvdan/sh, swappable).
- `internal/survey/` — parser-survey core (greppable diagnostics + exit code).
- `internal/wikidoc/`, `cmd/wikidoc/` — docs-sync tooling (not product code).
- `legacy/` — archived interactive plugin.

## Documentation

Canonical reader docs live on the **wiki**
(`ecosystem/plugins/zsh_lint`), which is the single reading surface. Code-derived
reference is generated from Go doc comments and synced into the wiki — do not
hand-edit the generated region there. Regenerate locally with:

    go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./internal/survey

## Writing Lint Rules
If you are implementing logic for the semantic analyzer, you must adopt the persona and guidelines specified in:
- `.github/agents/static-analysis-engineer.agent.md`
- `.github/instructions/go-ast-linting.instructions.md`

The root `README.md` is a minimal signpost for the GitHub landing page;
`docs/README.md` holds the repo-local pointers and contributor quickstart. Both
point at the wiki as the canonical reading surface.

## Build & test

    go build ./... && go vet ./... && go test ./...

Go 1.25 (`GOTOOLCHAIN=auto` auto-fetches the toolchain).
