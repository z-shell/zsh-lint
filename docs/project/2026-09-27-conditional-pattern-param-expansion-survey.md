# 2026-09-27 conditional pattern parameter expansion survey

Issue: [#513](https://github.com/z-shell/zsh-lint/issues/513).
Base: `440fbae0` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Glob Operators](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators) and [Parameter Expansion](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion); the rule is `in_brace_param` in `gettokstr`, `Src/lex.c`, at the `zsh-5.9.2` release.

## Change

Zsh counts no parentheses inside a parameter expansion, so in `[[ x == a(${x:-(}) ]]` the `(` in the default word opens no glob group.
Since #511 the parser fork reads it that way, but the active-pattern scan of the nested conditional pattern adapter (#120) still counted that `(` and reported ``unmatched `(` in conditional pattern``.
The scan now skips an unquoted `${...}` whole, using the shared `skipBracedParameter` rule, as it already did inside double quotes.
An expansion the rule cannot close is left to the existing checks.
No adapter was added, and no fork code changed.

## Rows

Rows 01-08 are the issue's rows; rows 09-24 are additional probes.
All are top-level probes.

| Row | Source                          | Native | Base   | Fixed  |
| --- | ------------------------------- | ------ | ------ | ------ |
| 01  | `[[ x == a(${x:-(}) ]]`         | accept | reject | accept |
| 02  | `[[ x == a(${x:-(}\|b) ]]`      | accept | reject | accept |
| 03  | `[[ x == (${x:-(}) ]]`          | accept | reject | accept |
| 04  | `[[ x == a(${(j:(:)y}) ]]`      | accept | reject | accept |
| 05  | `[[ x == a("${x:-(}") ]]`       | accept | accept | accept |
| 06  | `print a(${x:-(})`              | accept | accept | accept |
| 07  | `case x in a(${x:-(})) ;; esac` | accept | accept | accept |
| 08  | `[[ x == a(${x:-)}) ]]`         | accept | accept | accept |
| 09  | `[[ x == (a\|(b\|${x:-(})) ]]`  | accept | reject | accept |
| 10  | `[[ x == (a\|(b\|${x:-)})) ]]`  | accept | reject | accept |
| 11  | `[[ x == ((${x:-(})\|b) ]]`     | accept | reject | accept |
| 12  | `[[ x == a(${x:-${y:-(}}) ]]`   | accept | reject | accept |
| 13  | `[[ x == a(${x:-{(}}) ]]`       | accept | reject | accept |
| 14  | `[[ x == ${x:-(} ]]`            | accept | reject | accept |
| 15  | `[[ x == *${x:-(}* ]]`          | accept | reject | accept |
| 16  | `[[ x == (a\|(b)${x:-(}) ]]`    | accept | reject | accept |
| 17  | `[[ x == a(${x:-'}'}) ]]`       | accept | accept | accept |
| 18  | `[[ x == a(${x/(/y}) ]]`        | accept | reject | accept |
| 19  | `[[ x == (a\|(${x:-(}) ]]`      | reject | reject | reject |
| 20  | `[[ x == a(${x:-(} ]]`          | reject | reject | reject |
| 21  | `[[ x == a(${x};b) ]]`          | reject | reject | reject |
| 22  | `[[ x == (a\|(b\|${x});c) ]]`   | reject | reject | reject |
| 23  | `[[ x == (a\|(b\|${x) ]]`       | reject | reject | reject |
| 24  | `[[ x == (a\|(b\|${x:-{})) ]]`  | reject | accept | accept |

Row 24 is a false accept that predates this change; in a function body, a brace group or an `always` block this change now rejects it, as Zsh does, and elsewhere it remains accepted.
It is the same pre-existing family as `print ${x:-{}`, an unclosed brace in the word of a parameter expansion, which zsh-lint accepts on `main`.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 100 files compared, 99 unchanged, 1 `FIXED` (the new `ok-cond-pattern-param-expansion-paren.zsh`).
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: 333 unchanged.
- Probe grid: `zsh-lint-probe` over 12 bodies in every scanner context, 312 files: 130 `FIXED`, 5 `REJECTED` (row 24 in brace-closed contexts, native-invalid), 177 unchanged, no `REGRESSED` and no `FALSE-ACCEPT`. The 21 known false accepts are row 24 in the remaining contexts, accepted on base too.
- Adversarial rows: 120 generated rows (six contexts by twenty expansion bodies) and 42 hand rows, with no introduced false accept or regression.
- Corpus fixture: passes `zsh -f -n` and runs under `zsh -f`, printing all eight match lines.
- Hand mutants of the new gate: 3 of 6 killed. Row 17 parses on base as well, so it does not guard the fix itself; it is in the unit test because it kills the mutant that scans the expansion in double-quote mode, where the `'` would not quote and the `}` would close the expansion early.
  The three survivors (dropping the `{` test, dropping the `+ 1`, ignoring `ok`) change no verdict over a 700-row generated grid: the skip only chooses what the scan counts, and the parser still reads the whole source. The code comment records this.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
