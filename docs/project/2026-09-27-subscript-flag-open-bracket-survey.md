# 2026-09-27 subscript flag open bracket survey

Issue: [#534](https://github.com/z-shell/zsh-lint/issues/534).
Base: `5a3b2efc` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags).
Source: `dquote_parse` in `Src/lex.c`.

## Change

Zsh reads a `${...}` subscript with `dquote_parse`, which counts the subscript's unescaped `[` bytes, quoted ones included. A `]` only ends the subscript once none is open, so in `${x[(r)a[]}` the subscript never closes and Zsh reports an invalid subscript.

The raw subscript-flag argument reader (`zshSubFlags`) stops at the first `]`. The flag-pattern adapters then read a balanced bracket expression such as `a[b]c`, but nothing reported a `[` that was never closed. So `${x[(r)a[]}`, `${x[(r)[]}` and `${x[(r)'['a]}` were accepted.

The reader now counts the argument's own `[` bytes in a `${...}` subscript:

- escaped ones are skipped;
- so are the brackets of a nested `${...}`, which has its own subscripts, in double quotes too (rows 22 and 23), and those of a short subscript (#532);
- quoted ones are counted, as Zsh counts them.

If one is still open where the reader stops at the subscript's closing `]`, the one right before the expansion's `}`, the first open `[` is reported. A `]` the reader stops at inside the argument, as in `a[b]c`, and a `[` open at a `,` are left to the flag-pattern adapters as before.

#532 estimated that counting top-level brackets would change 13 test groups of those adapters. Reporting only the `[` left open at the subscript's end changes two existing test rows, both of which pinned the native-invalid `${m[(r)a[b]}` as accepted. The maintainer approved changing both.

The check applies in every context, as for the earlier subscript fixes. In an assignment or a `[[ ]]` operand `zsh -f -n` does not check the subscript, so rows 18 and 19 pass `zsh -f -n` and fail when run with `invalid subscript`.

## Rows

All are top-level probes. `$x`, `$y` and `$m` are arrays in the runtime checks.

| Row | Source                       | Native | Base   | Fixed  |
| --- | ---------------------------- | ------ | ------ | ------ |
| 01  | `print ${x[(r)a[]}`          | reject | accept | reject |
| 02  | `print ${x[(r)[]}`           | reject | accept | reject |
| 03  | `print ${x[(i)[]}`           | reject | accept | reject |
| 04  | `print ${x[(r)a[b]}`         | reject | accept | reject |
| 05  | `print ${x[(r)'['a]}`        | reject | accept | reject |
| 06  | `print ${x[(r)"["]}`         | reject | accept | reject |
| 07  | `print "${x[(r)a[]}"`        | reject | accept | reject |
| 08  | `print ${x[(r)'$y[(r)a]'[]}` | reject | accept | reject |
| 09  | `print ${x[(r)${y}[]}`       | reject | accept | reject |
| 10  | `print ${x[(r)a[b]c]}`       | accept | accept | accept |
| 11  | `print ${x[(r)[a],2]}`       | accept | accept | accept |
| 12  | `print ${x[(r)[[:alpha:]]]}` | accept | accept | accept |
| 13  | `print ${x[(r)a[]b]}`        | accept | accept | accept |
| 14  | `print ${x[(r)\[]}`          | accept | accept | accept |
| 15  | `print ${m[(i)${Y[a]}]}`     | accept | accept | accept |
| 16  | `print ${m[(I)[${y[2]}]]}`   | accept | accept | accept |
| 17  | `print ${x[(r)a[],2]}`       | accept | accept | accept |
| 18  | `x=${x[(r)a[]}`              | accept | accept | reject |
| 19  | `[[ ${x[(r)a[]} == a ]]`     | accept | accept | reject |
| 20  | `print $x[(r)a[]`            | reject | accept | accept |
| 21  | `print ${x[(r)a[b][]}`       | reject | reject | reject |
| 22  | `print "${m[(i)${y}[]}"`     | reject | accept | reject |
| 23  | `print "${m[(I)[${y[2]}]]}"` | accept | accept | accept |

Row 16 is the shape of `Functions/Zle/select-bracketed` in the vendored Zsh tree, which an earlier draft rejected; the nested `${...}` is now skipped. Row 20 was left as it was; it is fixed by [#538](https://github.com/z-shell/zsh-lint/issues/538), see the [short subscript flag open bracket survey](2026-09-27-short-subscript-flag-open-bracket-survey.md).

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 85 files of `z-shell/F-Sy-H`: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 7980 rows (19 argument parts in pairs, 3 flags, 7 contexts): 270 fixed false accepts and no introduced false accept.
  261 rows are newly rejected, all assignments or `[[ ]]` operands as in rows 18 and 19, and all fail when run.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 14 of 14 killed, after rows for four first-round survivors were added. One of them showed nested expansions in double quotes were counted twice, now fixed (row 22).
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
