# 2026-09-27 short subscript flag open bracket survey

Issue: [#538](https://github.com/z-shell/zsh-lint/issues/538).
Base: `f536c0c0` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags).
Source: `dquote_parse` and `gettokstr` in `Src/lex.c`.

## Change

[#534](https://github.com/z-shell/zsh-lint/issues/534) reports a `[` in a `${...}` subscript flag argument that is still open at the subscript's closing `]`. A short `$x[...]` has no closing `}`, so the check did not apply there, and `$x[(r)a[]` was still accepted.

Zsh parses a short subscript when the word is expanded, counting every unescaped `[` and `]` of the subscript, quoted ones included, across the whole lexical word. The subscript's `]` is the one that closes the last open `[`; if the word ends first, the subscript is invalid. So `$x[(r)a[b]c]` is valid, with `a[b]c` as the pattern, and `$x[(r)a[b] c]` is not. The #534 survey's note that a short subscript ends at its first `]` was wrong and is corrected there.

The flag-pattern adapters already read a balanced pattern past the first `]`. When the raw subscript-flag argument reader (`zshSubFlags`) stops at a `]` in a short subscript with a `[` of its own still open, it now reads ahead in the word, without consuming, to see whether a later `]` closes it:

- a blank, a newline, `;`, `&`, `|` or a redirection ends the word, except inside a glob group of the argument, a nested `${...}` or quotes; a `)` that closes nothing ends it too;
- a numeric glob such as `<1-2>` is text, so its `>` is not a redirection;
- in double quotes the word goes on past the closing `"`, as in `"$x[(r)[a]"c"]"`;
- brackets inside a nested `${...}`, `$(...)` or backquote are their own, as in `${y:-]}`;
- a word too long for the read buffer is taken to close, so nothing is reported.

If no later `]` closes it, the first open `[` is reported with the #534 error. No existing test row changed.

The check applies in every context, as for the earlier subscript fixes. In an assignment `zsh -f -n` does not check the subscript, and a command substitution's body is only parsed when run, so rows 22 and 23 pass `zsh -f -n` and fail when run with `invalid subscript`.

## Rows

All are top-level probes. `$x` and `$y` are arrays in the runtime checks.

| #   | Source                       | zsh -f -n | base   | fixed  | run               |
| --- | ---------------------------- | --------- | ------ | ------ | ----------------- |
| 1   | `print $x[(r)a[]`            | reject    | accept | reject | invalid subscript |
| 2   | `print $x[(r)[]`             | reject    | accept | reject | invalid subscript |
| 3   | `print $x[(r)a[b]`           | reject    | accept | reject | invalid subscript |
| 4   | `print $x[(r)'['a]`          | reject    | accept | reject | invalid subscript |
| 5   | `print $x[(r)a[b] c]`        | reject    | accept | reject | invalid subscript |
| 6   | `print $x[(r)a[b];c]`        | reject    | accept | reject | invalid subscript |
| 7   | `print $x[(r)(a[)]`          | reject    | accept | reject | invalid subscript |
| 8   | `print "$x[(r)a[]"`          | reject    | accept | reject | invalid subscript |
| 9   | `print "$x[(r)a[]" "]"`      | reject    | accept | reject | invalid subscript |
| 10  | `print $x[(r)a[b]<1-2> c]`   | reject    | accept | reject | invalid subscript |
| 11  | `print $x[(r)a[b]${y:-]}`    | reject    | accept | reject | invalid subscript |
| 12  | `print $x[(r)a[b]$(echo ])`  | reject    | accept | reject | invalid subscript |
| 13  | `print $x[(r)a[b]c]`         | accept    | accept | accept | none              |
| 14  | `print $x[(r)a[]]`           | accept    | accept | accept | none              |
| 15  | `print $x[(r)a[],2]`         | accept    | accept | accept | none              |
| 16  | `print $x[(r)[[:alpha:]]]`   | accept    | accept | accept | none              |
| 17  | `print $x[(r)\[]`            | accept    | accept | accept | none              |
| 18  | `print "$x[(r)[a]"c"]"`      | accept    | accept | accept | none              |
| 19  | `print $x[(r)(a[b] \|c)]`    | accept    | accept | accept | none              |
| 20  | `print $x[(r)[a]<1-2>]`      | accept    | accept | accept | none              |
| 21  | `print $x[(r)a[b]$(echo ])]` | accept    | accept | accept | none              |
| 22  | `x=$x[(r)a[]`                | accept    | accept | reject | invalid subscript |
| 23  | `print $(print $x[(r)a[])`   | accept    | accept | reject | invalid subscript |

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 85 files of `z-shell/F-Sy-H`: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 9060 rows (28 argument parts in pairs, 2 flags, 6 contexts): 786 fixed false accepts and no introduced false accept. 620 rows are newly rejected, all assignments, command substitution bodies or `[[ ]]` operands as in rows 22 and 23, and all fail when run. An earlier draft rejected valid words ending in a numeric glob, found by the grid; they are test rows now.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 26 of 26 killed, after rows for nine first-round survivors were added (among them a word at the end of input, a word longer than the read buffer, and backquote and process substitution bodies). A tenth survivor was equivalent: its guard was implied by the count before it, so it was removed from the code.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
