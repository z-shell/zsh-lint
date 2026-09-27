# 2026-09-27 parameter expansion word braces survey

Issue: [#518](https://github.com/z-shell/zsh-lint/issues/518).
Base: `cc5beb10` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 5 zsh -f` where `-n` cannot decide.
Manual: [Parameter Expansion](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion); the rule is `bct` and `in_brace_param` in `gettokstr`, `Src/lex.c`, at the `zsh-5.9.2` release.

## Change

Outside double quotes, Zsh nests braces in the word of a parameter expansion: in `${x:-{a}b}` the first `}` closes the `{`, and in `${x:-{}` the only `}` closes the `{`, leaving the expansion open.
Inside double quotes, and in a here-document body, the first `}` closes the expansion.
The parser fork closed every expansion at its first `}`, which rejected `${x:-{}}` and accepted `${x:-{}`.

The fork now counts `{` bytes in the word of an unquoted Zsh expansion (`zshParamBraces`), for every operator that takes a word.
A nested `${...}` starts its own count; the enclosing count is restored before the token after the inner `}` is read, because that token may itself be a `}` of the enclosing word.
A short `$name` has no word and leaves the count alone.

Two changes to the #400 double-quote state were needed for the vendored Zsh completion tree, which the first draft regressed.
A nested expansion in a double-quoted string inherits that state, so `"${${(%):-%F{$c\}}#?\[}"` (`Completion/Zsh/Type/_ps1234`) keeps closing at the first `}`.
A here-document body counts as double-quoted, which runtime checks confirm: in a here-document, `${u:-{}` prints `{` and `${u:-'}'}` closes at the first `}`.
Every change is behind `LangZsh`.

## Rows

Rows 01-12 are the issue's rows; rows 13-30 are additional probes.
All are top-level probes unless marked.

| Row | Source                      | Native | Base   | Fixed  |
| --- | --------------------------- | ------ | ------ | ------ |
| 01  | `print ${x:-{}`             | reject | accept | reject |
| 02  | `print ${x:-a{b}`           | reject | accept | reject |
| 03  | `x=${x:-{}`                 | reject | accept | reject |
| 04  | `print ${x:-{}}`            | accept | reject | accept |
| 05  | `print "${x:-{}"`           | accept | accept | accept |
| 06  | `print ${x:-a{b}c}`         | accept | reject | accept |
| 07  | `print ${x:-{a}b}`          | accept | reject | accept |
| 08  | `print ${x/a/{}}`           | accept | reject | accept |
| 09  | `print ${x/{}/b}`           | accept | reject | accept |
| 10  | `print ${x#{}}`             | accept | reject | accept |
| 11  | `print ${x/{/b}`            | reject | accept | reject |
| 12  | `print ${x#{}`              | reject | accept | reject |
| 13  | `print ${x:-${y:-{}}}`      | accept | reject | accept |
| 14  | `print ${x:-{$y}}`          | accept | reject | accept |
| 15  | `print ${x:-{${y}}}`        | accept | reject | accept |
| 16  | `print ${x:-{${y:-a}}}`     | accept | reject | accept |
| 17  | `print ${x:-${y:-{}}`       | reject | accept | reject |
| 18  | `print ${x:-{$y}`           | reject | accept | reject |
| 19  | `print ${x:-\{}`            | accept | accept | accept |
| 20  | `print ${x:-'{'}`           | accept | accept | accept |
| 21  | `[[ ${x:-{}} == a ]]`       | accept | reject | accept |
| 22  | `f() { print ${x:-{}}; }`   | accept | reject | accept |
| 23  | `print ${x:-{} ; print y }` | accept | reject | accept |
| 24  | `x="${${x:-{}}"`            | accept | accept | accept |
| 25  | `x="${${x:-{a\}}#a}"`       | accept | accept | accept |
| 26  | `print "${${x:-'}'}}"`      | reject | accept | reject |
| 27  | `x="${${x:-'}'}}"`          | accept | accept | reject |
| 28  | `print ${x:\|{}}`           | reject | reject | accept |
| 29  | `print ${x#{(}}`            | reject | reject | accept |
| 30  | `print ${x[{]}`             | reject | accept | accept |

Row 27 is a runtime-tier rejection: `zsh -f -n` does not expand an assignment word (#287), and running it reports `bad substitution`, the same error as its command-word form, row 26.
Rows 28 and 29 are runtime-tier acceptances: `zsh -f -n` rejects them at the top level with an expansion error (`not an identifier: {}`, `bad pattern: {(}`) and accepts the same text in a function body, where only the lexer runs.
Base rejected them for a lexer reason that was wrong for Zsh, the stray `}`; the fixed parser reads them as Zsh's lexer does.
Row 30, an unclosed `{` in a subscript, is a separate false accept that this change leaves in place.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 101 files compared, 100 unchanged, 1 `FIXED` (the new `ok-param-word-nested-braces.zsh`).
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: 333 unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged. A first draft regressed `Completion/Redhat/Command/_rpm` and `Completion/Zsh/Type/_ps1234`, which the corpus and the workspace missed; the nested double-quote rule fixes both.
- Generated grid: 6720 rows (16 operators, 28 word shapes, 15 contexts including double quotes, nesting and both here-document forms): 1224 fixed rejections, 880 fixed acceptances, and no change in the rest except two classes, both runtime-tier.
  16 rows are row 27's shape, one per operator, now rejected as Zsh does when it runs them.
  120 rows are rows 28 and 29's shape, now accepted because only expansion rejects them.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 16 of 16 killed, including each exit that restores the enclosing count and both here-document quote states.
- Corpus fixture: passes `zsh -f -n` and runs under `zsh -f`, printing the nested-brace values Zsh gives.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
