# 2026-09-27 subscript flag substitution bracket survey

Issue: [#529](https://github.com/z-shell/zsh-lint/issues/529).
Base: `e05ae53f` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags) and [Command Substitution](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution).
Source: `dquote_parse` and `parse_subscript` in `Src/lex.c`.

## Change

Since #526 the raw subscript-flag argument reader follows a command substitution in the argument and parses its body.
A `]` inside that substitution still ended the argument, so valid Zsh such as `${x[(r)$(echo [a])]}` or `${x[(r)$((x[1]+2))]}` was rejected.

A `]` inside a `$(...)` or `$((...))` body is now text of the body.
How Zsh treats the brackets there depends on where the subscript is:

- In a `${...}` subscript, Zsh reads the subscript with `dquote_parse` and counts its `[` and `]` bytes, including those inside a command substitution, where quotes and backslashes do not hide them: `$(echo \[)` is rejected and `$(echo '[')$(echo \])` is valid. The expansion is rejected unless they balance over the whole subscript. So `$(echo ])$(echo [)` is valid and `$(echo [)` is not. The parser now requires the same balance, quoted and escaped bytes in `$(...)` bodies included, and reports an unbalanced one at its first bracket.
- A short `$y[...]` inside such a subscript is counted with it.
- In a short `$x[...]`, an assignment's `a[...]`, and a `${...}` inside a short subscript, Zsh does not count them, so they are text.
- A backquoted body is read by Zsh on its own. In a `${...}` its unescaped brackets are counted separately, and a `]` there must close an earlier `[` of the same body. In a short `$x[...]` a `]` that closes an earlier `[` stays in the body, so ``$x[(r)`echo [a]`]`` no longer fails with an unclosed backquote.

At a `,`, only a `]` that no earlier `[` opened is an error: `$(echo [),$(echo ])` is valid Zsh, and the part after the `,` is parsed elsewhere.

Seven existing adapter test rows had pinned native-valid sources such as `${line[(i)$(echo [x])]}` as rejected, the #237 limit of the flag-pattern scanners. The parser now accepts them without the scanners, and the maintainer approved changing those rows to expect acceptance. Two native-invalid rows keep their rejection with the new message and column.

The check applies in every context, as for #519 and #526.
Where `zsh -f -n` does not parse a subscript, in an assignment or inside a command substitution, an unbalanced one passes `zsh -f -n` and fails when run (rows 22 and 23).

## Rows

All are top-level probes.

| Row | Source                               | Native | Base   | Fixed  |
| --- | ------------------------------------ | ------ | ------ | ------ |
| 01  | `print ${x[(r)$(echo [a])]}`         | accept | reject | accept |
| 02  | `print ${x[(r)$((x[1]+2))]}`         | accept | reject | accept |
| 03  | `print ${x[(r)$(echo ${y[1]})]}`     | accept | reject | accept |
| 04  | `print ${x[(r)$(echo ])$(echo [)]}`  | accept | reject | accept |
| 05  | `print ${x[(r)$(echo [),$(echo ])]}` | accept | accept | accept |
| 06  | `print ${x[(r)$(echo [a]),2]}`       | accept | reject | accept |
| 07  | `print $x[(r)$(echo ])]`             | accept | reject | accept |
| 08  | `print "$x[(r)$(echo ])]"`           | accept | reject | accept |
| 09  | `a[(r)$(echo ])]=b`                  | accept | reject | accept |
| 10  | `print $x[${y[(r)$(echo [)]}]`       | accept | accept | accept |
| 11  | `print ${x[$y[(r)$(echo [a])]]}`     | accept | reject | accept |
| 12  | ``print ${x[(r)`echo [a]`]}``        | accept | reject | accept |
| 13  | ``print $x[(r)`echo [a]`]``          | accept | reject | accept |
| 14  | `print ${m[(r)$(echo [x])##]}`       | accept | reject | accept |
| 15  | `print ${x[(r)$(echo ])]}`           | reject | reject | reject |
| 16  | `print ${x[(r)$(echo [)]}`           | reject | accept | reject |
| 17  | `print ${x[(r)$(echo "[")]}`         | reject | accept | reject |
| 18  | `print ${x[(r)$(echo ]),2]}`         | reject | reject | reject |
| 19  | `print ${x[$y[(r)$(echo ])]]}`       | reject | reject | reject |
| 20  | ``print ${x[(r)`echo ][`]}``         | reject | reject | reject |
| 21  | ``print ${x[(r)`echo [`$(echo [)]}`` | reject | accept | reject |
| 22  | `x=${x[(r)$(echo [)]}`               | accept | accept | reject |
| 23  | `print $(echo ${y[(r)$(echo [)]})`   | accept | accept | reject |
| 24  | `print ${x[(r)$(echo [),2]}`         | reject | accept | accept |
| 25  | `print ${x[(r)$y[(r)$(echo [a])]]}`  | accept | reject | reject |

Rows 22 and 23 are rejected under the every-context choice: `zsh -f -n` does not parse these subscripts, and running each reports `bad substitution`.
Two rows are gaps left as they were:

- Row 24: a `[` opened before a `,` and never closed is accepted, because the text after the `,` is parsed by the arithmetic reader, which does not see the count.
- Row 25: a flag subscript nested in a flag argument of a `${...}` still ends at its first `]`, as on base.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 102 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 1487 files of the corpus audit tree: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 28560 rows (37 bodies, 10 substitution forms, 6 flags, 13 contexts): 4080 fixed false rejects, 780 fixed false accepts and no introduced false accept.
  324 rows are newly rejected, all under the every-context choice above, and all fail when run with `bad substitution` or `invalid subscript`.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 23 of 23 killed, after rows for three first-round survivors were added.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
