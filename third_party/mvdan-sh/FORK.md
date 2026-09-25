# mvdan.cc/sh fork

This directory is a fork of [`mvdan.cc/sh/v3`](https://github.com/mvdan/sh) at `v3.14.1`, kept under its upstream BSD-3-Clause license (`LICENSE`), as ADR-0030 in z-shell/.github decides. `go.mod` at the repository root replaces the module with this directory, so import paths are unchanged.

Only the packages zsh-lint builds are kept: `syntax`, `expand`, `pattern`, `fileutil`, and `internal`. Upstream's tests for them are kept and run in Go CI; they must keep passing.

## Local changes

Every change is Zsh-only, behind `LangZsh`, and listed here with the zsh-lint issue that made it.

| Change | Files | Issue |
| --- | --- | --- |
| Brace-form `if`/`elif`/`else` and `while`/`until` bodies (Alternate Forms For Complex Commands) | `syntax/parser.go` | [#446](https://github.com/z-shell/zsh-lint/issues/446) |

## Taking an upstream release

Copy the new release's packages over this directory, reapply the changes above, and run upstream's tests here and zsh-lint's full suite, with the tree-shape checks ADR-0023 point 3 requires.
