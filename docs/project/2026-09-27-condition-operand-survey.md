# 2026-09-27 condition operand survey

Issue: [#512](https://github.com/z-shell/zsh-lint/issues/512).
Base: `8b89d45f2a5b5ddaf59de6fdd14c3a334fdf1f68` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Conditional Expressions](https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html); the rules are `par_cond_2` and `par_cond_multi` in `Src/parse.c` at the `zsh-5.9.2` release.

## Change

The parser fork rejects two Zsh conditions it accepted:

- A lone unquoted `-` as a whole condition, which Zsh reads as a condition name with nothing after the dash (`condition expected: -`).
  A quoted or escaped `-`, `--`, and a `-` used as an operand (`[[ -n - ]]`, `[[ x == - ]]`) stay valid.
- An unquoted `!` or a word starting with `(` as the right operand of `<` or `>`, where `par_cond_2` requires a plain string.
  After `==`, `!=` and `=~` the operand is a pattern, so `[[ x == ! ]]` stays valid.

A newline may now separate `<` or `>` from its right operand, as in Zsh.
A `]]` there, on the same line or the next, is reported at the operator (`` `<` must be followed by a word ``).
Every change is behind `LangZsh`; Bash keeps its verdicts.
No compatibility adapter was added.

## Rows

Rows 01-13 are the issue's rows; rows 14-33 are additional probes.
All are top-level probes.

| Row | Source                    | Native | Base   | Fixed  |
| --- | ------------------------- | ------ | ------ | ------ |
| 01  | `[[ - ]]`                 | reject | accept | reject |
| 02  | `[[ ! - ]]`               | reject | accept | reject |
| 03  | `[[ ( - ) ]]`             | reject | accept | reject |
| 04  | `[[ -n a && - ]]`         | reject | accept | reject |
| 05  | `[[ x < ! ]]`             | reject | accept | reject |
| 06  | `[[ x > ! ]]`             | reject | accept | reject |
| 07  | `[[ x < (a) ]]`           | reject | accept | reject |
| 08  | `[[ x < ( \|\| ) ]]`      | reject | accept | reject |
| 09  | `[[ -- ]]`                | accept | accept | accept |
| 10  | `[[ x < y ]]`             | accept | accept | accept |
| 11  | `[[ x < "!" ]]`           | accept | accept | accept |
| 12  | `[[ x == ! ]]`            | accept | accept | accept |
| 13  | `[[ x < ( ]]`             | reject | reject | reject |
| 14  | `[[ - && x ]]`            | reject | accept | reject |
| 15  | `[[ x \|\| - ]]`          | reject | accept | reject |
| 16  | `[[ ! ! - ]]`             | reject | accept | reject |
| 17  | `[[ -\n]]`                | reject | accept | reject |
| 18  | `[[ x < (a)b ]]`          | reject | accept | reject |
| 19  | `[[ x > ( a ) ]]`         | reject | accept | reject |
| 20  | `[[ x <\ny ]]`            | accept | reject | accept |
| 21  | `[[ x < !y ]]`            | accept | accept | accept |
| 22  | `[[ x < a(b) ]]`          | accept | accept | accept |
| 23  | `[[ x > '(' ]]`           | accept | accept | accept |
| 24  | `[[ x < \! ]]`            | accept | accept | accept |
| 25  | `[[ x < <(a) ]]`          | accept | accept | accept |
| 26  | `[[ x < !(a) ]]`          | accept | accept | accept |
| 27  | `[[ x -nt (a) ]]`         | accept | accept | accept |
| 28  | `[[ "-" ]]`               | accept | accept | accept |
| 29  | `[[ \- ]]`                | accept | accept | accept |
| 30  | `[[ - == x ]]`            | accept | accept | accept |
| 31  | `[[ -n - ]]`              | accept | accept | accept |
| 32  | `[[ -prefix - && -n x ]]` | accept | accept | accept |
| 33  | `[[ - a ]]`               | reject | reject | reject |

In rows 17 and 20, `\n` stands for a newline in the file.
Row 20 is an incidental fix: a newline after `<` or `>` is now skipped, as it is after the other binary operators.

Two review follow-ups on the first head: `[[ x <` newline `]]` reported its error at the `[[` after the newline skip, and now reports it at the `<` again, as base did.
The same check fixes a pre-existing false accept, `[[ x < ]] ]]`, where base read the first `]]` as the operand.
The #484 row `[[ -prefix < ]]` keeps its rejection and now reports ``1:12: `<` must be followed by a word`` instead of an unmatched `[[` at 1:1; Zsh reports `near ]]`, just after that operator.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 99 files compared, 99 unchanged.
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: 333 unchanged.
- Probe rows: 64 condition rows judged three ways; 17 intended fixes, 1 incidental fix, no introduced false accept, no regression.
- Crash probe: 532 rows with each follow-up token after `<`, `>`, a lone `-`, `!`, `(` and `||`, under `timeout 5`: no panic and no hang.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 19 of 19 killed, each `LangZsh` gate forced both ways and the `]]` check forced off and widened.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
