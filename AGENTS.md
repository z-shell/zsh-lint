# Project Guidelines — zsh-lint

This project follows the organization-wide [Z-Shell Organization Guidelines](https://github.com/z-shell/.github/blob/main/AGENTS.md).

## What this is

`zsh-lint` is a **Go-based semantic analyzer for Zsh** (see [#5](https://github.com/z-shell/zsh-lint/issues/5)).
The active product is the Go code under `cmd/` and `internal/`; the parser front end uses [`mvdan/sh`](https://github.com/mvdan/sh).
The CLI runs a default set of static-analysis rules and reports greppable diagnostics.

The original interactive Zi/`.zshrc` plugin was removed from the tree; the Go analyzer is the whole product surface.

## Branch model

- `main` is the development branch; short-lived feature and fix branches start from and target `main`.
- Use `feature-<id>`, `bug-<id>`, or `hotfix-<id>` branch names.
- Merge reviewed pull requests into `main` with squash merge.
- Annotated `vX.Y.Z` tags are the publication boundary.

## Layout

- `cmd/zsh-lint/` — semantic-analyzer CLI entry point.
- `cmd/zsh-lint-survey/` — parser-gap survey CLI entry point.
- `internal/parse/` — parser front end (mvdan/sh, swappable).
- `internal/survey/` — parser-survey core (greppable diagnostics + exit code).
- `internal/wikidoc/`, `cmd/wikidoc/` — docs-sync tooling (not product code).

## Documentation

Canonical reader docs live on the **wiki** (`community/zsh_lint`), which is the single reading surface.
Code-derived reference is generated from Go doc comments and synced into the wiki — do not hand-edit the generated region there.
Regenerate locally with:

    go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./cmd/zsh-lint-survey ./internal/survey ./internal/rules

## Scoped guidance

Read the matching file before changing code under its path; Copilot loads them by `applyTo`, other runtimes must open them explicitly.

- `.github/instructions/go-ast-linting.instructions.md` for rules and the analyzer (`internal/analyzer/`, `internal/rules/`): visitor pattern, safe text extraction, table-driven tests.
  Rule intake follows `docs/project/rule-policy.md`.
- `.github/instructions/parser-front-end.instructions.md` for the parser (`internal/parse/`, `internal/survey/`): dual-oracle proof, adapter invariants, fixture naming.
  The full contract is `docs/project/parser-gap-workflow.md`.

`docs/project/README.md` separates the living contracts from dated survey reports.

### Markdown line breaks

Write prose with **semantic line breaks**: one sentence per line, breaking a long sentence at a clause boundary.
Do not hard-wrap to a column.

This is not cosmetic.
GitHub renders Markdown with GFM line breaks, so every newline inside a paragraph becomes a `<br>`.
Column-wrapped prose therefore renders with breaks at arbitrary mid-sentence positions, while semantic breaks put them at sentence ends where they read as intended.
Editing a sentence also touches only its own line, so diffs stay reviewable instead of reflowing a whole paragraph.

Nothing enforces this.
`prettier` runs unconfigured, so `proseWrap: preserve` leaves prose untouched, and `markdownlint` runs with MD013 off.
It is a convention maintained by hand and in review.

Leave tables, code fences, link definitions, and frontmatter alone.

The root `README.md` is the GitHub landing page, shaped by the organization README template: it summarizes the CLI, configuration, output, and exit-code contracts and must stay verified against `cmd/zsh-lint` and `docs/project/*.md`.
`docs/README.md` holds the repo-local pointers and contributor quickstart.
Both point at the wiki as the canonical reading surface.

## Build & test

    go build ./... && go vet ./... && go test ./...
    golangci-lint run ./...

Go CI runs `golangci-lint` v2.12.2 over the whole module with `.golangci.yml`, so a finding anywhere fails the build, not only on changed lines.

Go 1.26 (`GOTOOLCHAIN=auto` auto-fetches the toolchain; CI pins the same version explicitly). mvdan/sh v3.14 dropped Go 1.25.
