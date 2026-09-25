# mvdan.cc/sh fork

This directory is a fork of [`mvdan.cc/sh/v3`](https://github.com/mvdan/sh) at `v3.14.1`, kept under its upstream BSD-3-Clause license (`LICENSE`), as ADR-0030 in z-shell/.github decides. `go.mod` at the repository root replaces the module with this directory, so import paths are unchanged.

Only the packages zsh-lint builds are kept: `syntax`, `expand`, `pattern`, `fileutil`, and `internal`. Upstream's tests for them are kept and run in Go CI; they must keep passing.

## Local changes

Every change is Zsh-only, behind `LangZsh`, and listed here with the zsh-lint issue that made it.

| Change | Files | Issue |
| --- | --- | --- |
| Brace-form `if`/`elif`/`else` and `while`/`until` bodies (Alternate Forms For Complex Commands) | `syntax/parser.go` | [#446](https://github.com/z-shell/zsh-lint/issues/446) |
| A `'` in the word of a double-quoted `${...}` is text, not a single-quote opener | `syntax/parser.go`, `syntax/lexer.go` | [#400](https://github.com/z-shell/zsh-lint/issues/400) |
| In a glob group or glob qualifier, a quoted `)` and the `)` closing a command substitution do not end the group | `syntax/parser.go` | [#439](https://github.com/z-shell/zsh-lint/issues/439) |
| A case pattern whose leading group is glued to more pattern text, `(x)y)`, and a leading `((`, as in `((x\|y)\|z)` | `syntax/parser.go` | [#452](https://github.com/z-shell/zsh-lint/issues/452) |
| The alternate and short forms of `for` and `select` (several names, a `( word ... )` list, a `{ list }` or one-sublist body, an empty body), the short forms of `if`, `while` and `until`, `else` reserved in command position, and a `ZshEnd` position on `ForClause`, `IfClause` and `WhileClause` so `End()` is exact for those forms | `syntax/parser.go`, `syntax/nodes.go`, `syntax/parser_test.go` | [#459](https://github.com/z-shell/zsh-lint/issues/459) |

## Taking an upstream release

Copy the new release's packages over this directory, reapply the changes above, and run upstream's tests here and zsh-lint's full suite, with the tree-shape checks ADR-0023 point 3 requires.
