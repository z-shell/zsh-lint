# 2026-09-27 subscript bracket balance survey

Issue: [#531](https://github.com/z-shell/zsh-lint/issues/531).
Base: `fc9ef79f` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags) and [Command Substitution](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution).
Source: `dquote_parse` and `parse_subscript` in `Src/lex.c`.

## Change

Since #529 the raw subscript-flag argument reader requires the `[` and `]` bytes inside the command substitutions of a `${...}` subscript to balance, as Zsh does. It could check only the part before a `,`, and only in a flag argument, since the rest of the subscript is parsed as arithmetic. So a `[` still open at the `,` was accepted, as in `${x[(r)$(echo [),2]}`, and so was any unbalanced bracket in an unflagged subscript, as in `${x[1,$(echo [)]}`.

The check now runs over the whole subscript once it is parsed:

- While a `${...}` subscript is read, the lexer records every byte it consumes, dropped ones included, so the record lines up with the source.
- Once the subscript's `]` is matched, its text is scanned as Zsh counts it:
  - The `[` and `]` bytes of every `$(...)` and `$((...))` body count, quoted or escaped ones included.
  - So do those of a `${...}` word, outside a double-quoted string there.
  - Those of a backquoted body count on their own, and a `]` there must close an earlier `[`.
- Each kind must balance over the whole subscript. An error is reported at the first counted bracket of the unbalanced kind.

Brackets outside such bodies are not counted here: the subscript's own bracket expressions are left to the flag-pattern adapters as before, and a `[` left open in the argument's text is #534. A short `$x[...]` does not count these brackets, as in #529. A subscript nested inside a subscript is checked on its own.

The check applies in every context, as for #519, #526 and #529. In an assignment or a `[[ ]]` operand `zsh -f -n` does not check the subscript, so rows 19 and 20 pass `zsh -f -n` and fail when run with `bad substitution`.

## Rows

All are top-level probes. `$x`, `$y` and `$z` are arrays in the runtime checks.

| Row | Source                                        | Native | Base   | Fixed  |
| --- | --------------------------------------------- | ------ | ------ | ------ |
| 01  | `print ${x[(r)$(echo [),2]}`                  | reject | accept | reject |
| 02  | `print ${x[(r)$(echo [),$(echo [)]}`          | reject | accept | reject |
| 03  | `print ${x[(r)a,$(echo ])]}`                  | reject | accept | reject |
| 04  | `print ${x[1,$(echo [)]}`                     | reject | accept | reject |
| 05  | `print ${x[$(echo [),2]}`                     | reject | accept | reject |
| 06  | `print ${x[1,$(echo \[)]}`                    | reject | accept | reject |
| 07  | ``print ${x[(r)`echo [`,2]}``                 | reject | accept | reject |
| 08  | ``print ${x[(r)$(echo [),`echo ]`]}``         | reject | accept | reject |
| 09  | `print ${x[1,${z:-[}]}`                       | reject | accept | reject |
| 10  | `print ${x[${Y[a]},$(echo ])]}`               | reject | accept | reject |
| 11  | `print "${x[1,$(echo [)]}"`                   | reject | accept | reject |
| 12  | `print ${x[(r)$(echo [),$(echo ])]}`          | accept | accept | accept |
| 13  | `print ${x[$(echo ]),$(echo [)]}`             | accept | accept | accept |
| 14  | `print ${x[(r)${z:-[},$(echo ])]}`            | accept | accept | accept |
| 15  | `print ${m[(i)${Y[a]}]}`                      | accept | accept | accept |
| 16  | `print ${x[(r)${z:-"["}]}`                    | accept | accept | accept |
| 17  | `print ${x[1,$(echo [a])]}`                   | accept | accept | accept |
| 18  | `print $x[1,$(echo [)]`                       | accept | accept | accept |
| 19  | `x=${x[1,$(echo [)]}`                         | accept | accept | reject |
| 20  | `[[ ${x[(r)$(echo [),2]} == a ]]`             | accept | accept | reject |
| 21  | `print ${x[(r)${z:-[}$(echo ])]}`             | accept | reject | reject |
| 22  | `print ${x[(r)$(echo [),${y[(r)$(echo ])]}]}` | accept | reject | reject |

Rows 21 and 22 are gaps left as they were: in a flag argument, the #529 check before the `,` still counts only command substitutions, so a `${...}` word there cannot balance one.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 85 files of `z-shell/F-Sy-H`: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 8478 rows (substitution, backquote and parameter-expansion bodies either side of a `,`, flagged and unflagged, 6 contexts): 2716 fixed false accepts and no introduced false accept.
  1797 rows are newly rejected, all assignments or `[[ ]]` operands as in rows 19 and 20, and all fail when run with `bad substitution` or `invalid subscript`.
- Error positions follow the source across lines, backslash-newlines and a refilled read buffer (tests in `syntax/subscript_bracket_balance_test.go`).
- Hand mutants of the fork change (the mutation script does not reach the fork module): 23 of 30 killed, after rows for the first-round survivors were added; a first-round survivor also exposed an off-by-one start offset after a leading backslash-newline, now fixed and tested. Six survivors are equivalent: they reset or bound the record differently without changing a verdict or position. The seventh drops the escape of a `\$` outside a body; every row that tells it apart is native-invalid and already accepted before this check runs (the #534 class), so pinning it would pin a false accept.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
