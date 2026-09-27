# 2026-09-27 subscript flag substitution body survey

Issue: [#526](https://github.com/z-shell/zsh-lint/issues/526).
Base: `520e0b90` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags) and [Command Substitution](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution).

## Change

The parser fork reads a subscript flag argument, as in `${x[(r)PATTERN]}`, as raw text up to its `,` or `]`.
A command substitution or backquoted command in the argument stayed text, so its body was never parsed as commands: `print ${x[(r)$(done)]}` was accepted while `print $(done)` was rejected.
Zsh parses the body in both places.

The raw reader now follows such a substitution as the glob group reader does since #519.
When it closes, its body is parsed on its own, and an error in it is reported at its place in the source.
While it is open, a `,` is text of the substitution rather than the end of the argument, so `${x[(r)$(echo ,)]}` is no longer rejected.
Quoting inside the body follows the shell: a `)` in quotes or after a backslash does not close it, and a backslash escapes a `"` but not a `'`.

Three points follow how Zsh reads the subscript:

- The subscript reads as a double-quoted string (`dquote_parse` in `Src/lex.c`), where a `'` is text, so `${x[(r)'$(done)']}` still holds a substitution. Only a backslash before the `$` stops one.
- The argument is a pattern, so `<(z)` in it is `<` and a group, not a process substitution: `${x[(i)<(z)]}` finds an element `<z`.
- An escaped `]` inside a backquoted body is text of the body. Elsewhere an escaped `]` still ends the argument, as before.

The check applies in every context, as for #519.
`zsh -f -n` does not parse a substitution body in a subscript flag argument outside a command word, and does not parse a backquoted one even there, so such a row passes `zsh -f -n` and is rejected here.
Every such row in the grid fails when run.
This follows the choice made for #519: the parser already rejects the same substitution without a flag, as in ``${x[`done`]}`` (row 18).

## Rows

All are top-level probes.

| Row | Source                             | Native | Base   | Fixed  |
| --- | ---------------------------------- | ------ | ------ | ------ |
| 01  | `print ${x[(r)$(done)]}`           | reject | accept | reject |
| 02  | `print ${x[(r)a$(done)]}`          | reject | accept | reject |
| 03  | `print ${x[(r)$(done),2]}`         | reject | accept | reject |
| 04  | `print $x[(r)$(done)]`             | reject | accept | reject |
| 05  | `print "${x[(r)$(done)]}"`         | reject | accept | reject |
| 06  | `print ${x[(r)'$(done)']}`         | reject | accept | reject |
| 07  | `print ${x[(r)$(echo ')'; done)]}` | reject | accept | reject |
| 08  | `print ${x[(r)$(echo \); done)]}`  | reject | accept | reject |
| 09  | `print ${x[(r)\\$(done)]}`         | reject | accept | reject |
| 10  | `print ${x[(r)$(echo ,)]}`         | accept | reject | accept |
| 11  | `print ${x[(r)$(echo ,),2]}`       | accept | reject | accept |
| 12  | `print ${x[(r)$(true)]}`           | accept | accept | accept |
| 13  | `print ${x[(r)$((1<<2))]}`         | accept | accept | accept |
| 14  | `print ${x[(r)\$(done)]}`          | accept | accept | accept |
| 15  | `print ${x[(i)<(done)]}`           | accept | accept | accept |
| 16  | ``print ${x[(r)`echo \]`]}``       | accept | accept | accept |
| 17  | ``print ${x[(r)`done`]}``          | accept | accept | reject |
| 18  | ``print ${x[`done`]}``             | accept | reject | reject |
| 19  | `[[ ${x[(r)$(done)]} == a ]]`      | reject | accept | reject |
| 20  | `print ${x[(r)$(echo [a])]}`       | accept | reject | reject |
| 21  | `print ${x[(r)$((echo a); done)]}` | reject | accept | accept |

Row 17 is one of the rows the check rejects under the #519 choice: `zsh -f -n` accepts it, and running it reports a parse error in the substitution.
Row 18 is the same substitution without a flag, which base already rejects.
Two rows are pre-existing gaps, unchanged here:

- Row 20: a `]` inside a substitution in a flag argument still ends the argument, so valid Zsh such as `$(echo [a])` or `$((x[1]+2))` there is rejected, as on base.
- Row 21: a `$((` that opens a subshell inside a command substitution is left as arithmetic, as in the glob group reader since #519 (`print a($((echo a); done))` is accepted too).

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 102 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 1487 files of the corpus audit tree: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 12960 rows (24 bodies, 9 substitution forms, 6 flags, 10 contexts): 2358 fixed false accepts, 336 fixed false rejects and no introduced false accept.
  1386 rows are newly rejected, all under the #519 choice above, and all fail when run (1260 with a parse error).
  768 of them hold a backquoted body, and 1013 are rejected on base when the flag is removed.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 18 of 18 killed.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
