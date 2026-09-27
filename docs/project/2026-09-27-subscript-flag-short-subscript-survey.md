# 2026-09-27 subscript flag short subscript survey

Issue: [#532](https://github.com/z-shell/zsh-lint/issues/532).
Base: `c2d3be40` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags).
Source: `dquote_parse` in `Src/lex.c`.

## Change

The raw subscript-flag argument reader (`zshSubFlags`) followed nested `${...}` expansions and command substitutions, but not a short subscript `$y[...]`.
In `${x[(r)$y[(r)a]]}` the `]` that closes `$y[(r)a]` ended the outer argument, so valid Zsh was rejected.

A short subscript with flags in the argument, as in `$y[(r)a]`, is now text of the argument:

- Its `[` and `]` are counted until its own `]`, escaped ones excepted, and a `,` inside it is its own rather than the outer range.
- A `{` inside it counts with the outer expansion, as Zsh counts it, and a stray `}` inside it is an error, since it closes the outer expansion in Zsh.
- A further `[` right after it, as in `$y[(r)a][1]`, opens another subscript of the same parameter.
- The bracket balance of #529 still applies inside it.

An unflagged short subscript such as `$y[1]` is left as before. The flag-pattern adapters (#382) already accept it by reading what follows, and taking it into the argument would change what they see.

The check applies in every context, as before. In a `[[ ]]` operand `zsh -f -n` does not parse the subscript, so row 17 passes `zsh -f -n` and fails when run with `invalid subscript`.

A `[` never closed in the argument's own text is still not counted (row 19, a false accept on base). Before this change, a short subscript that preceded one ended the argument early, so such a word was rejected by accident; now it is accepted like the rest of that class (row 18). The maintainer chose to keep this change narrow and file the class as [#534](https://github.com/z-shell/zsh-lint/issues/534).

## Rows

All are top-level probes. `$x`, `$y` and `$z` are arrays in the runtime checks.

| Row | Source                            | Native | Base   | Fixed  |
| --- | --------------------------------- | ------ | ------ | ------ |
| 01  | `print ${x[(r)$y[(r)a]]}`         | accept | reject | accept |
| 02  | `print ${x[(r)$y[(i)b]]}`         | accept | reject | accept |
| 03  | `print ${x[(r)$y[(r)a[b]c]]}`     | accept | reject | accept |
| 04  | `print ${x[(r)$y[(r)$z[(r)a]]]}`  | accept | reject | accept |
| 05  | `print ${x[(r)$y[(r)a]$z[(r)b]]}` | accept | reject | accept |
| 06  | `print ${x[(r)$y[(r)a][1]]}`      | accept | reject | accept |
| 07  | `print ${x[(r)$y[(r)a,2]]}`       | accept | reject | accept |
| 08  | `print ${x[(r)$y[(r)a],2]}`       | accept | reject | accept |
| 09  | `print ${x[(r)'$y[(r)a]']}`       | accept | reject | accept |
| 10  | `print ${x[(r)$y[(r)\]]]}`        | accept | reject | accept |
| 11  | `x=${x[(r)$y[(r)a]]}`             | accept | reject | accept |
| 12  | `print ${x[(r)$y[1]]}`            | accept | accept | accept |
| 13  | `print $x[(r)$y[(r)a]]`           | accept | accept | accept |
| 14  | `print ${x[(r)$y[(r)a]}`          | reject | accept | reject |
| 15  | `print ${x[(r)$y[(r)a]]x}`        | reject | reject | reject |
| 16  | `print ${x[(r)$y[(r)$(echo ])]]}` | reject | reject | reject |
| 17  | `[[ ${x[(r)$y[(r)a\]]} == a ]]`   | accept | accept | reject |
| 18  | `print ${x[(r)'$y[(r)a]'[]}`      | reject | reject | accept |
| 19  | `print ${x[(r)a[]}`               | reject | accept | accept |
| 20  | `print ${x[(r)\$y[(r)a]]}`        | accept | reject | reject |
| 21  | `print ${x[(r)${y}[(r)a]]}`       | accept | reject | reject |
| 22  | `print ${x[(r)$y[1][(r)a]]}`      | accept | reject | reject |

Rows 20 to 22 are gaps left as they were: an escaped `\$` before a subscript, a braced `${y}` followed by a subscript, and a flagged subscript chained after an unflagged one.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 85 files of `z-shell/F-Sy-H`: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 5670 rows (21 inner subscripts, 9 tails, 5 quote prefixes, 6 contexts): 1070 fixed false rejects and 99 fixed false accepts.
  21 rows are newly rejected, all `[[ ]]` operands with an escaped `\]` as in row 17, and all fail when run.
  70 rows are newly accepted false accepts, all with a `[` left open after the short subscript as in row 18. In 64 of them the same text with the short subscript replaced by a word is already accepted on base; the class is #534.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 18 of 19 killed, after rows for five first-round survivors were added. The survivor keeps the flag that follows a closed short subscript set for one more byte; the only rows it changes are native-invalid rows of the #534 class, which the change accepts and the mutant rejects, so a test would have to pin a false accept.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
