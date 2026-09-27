# 2026-09-27 subscript brace survey

Issue: [#521](https://github.com/z-shell/zsh-lint/issues/521).
Base: `3bb1084d` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 5 zsh -f` where `-n` cannot decide.
Manual: [Array Subscripts](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts); the rule is `bct` in `gettokstr`, `Src/lex.c`, at the `zsh-5.9.2` release.

## Change

Zsh lexes an unquoted `${...}` with one brace count, subscript included.
A balanced `{...}` in a subscript is part of the key, blanks and commas included: `${h[{a b}]}` reads the key `{a b}`.
An unclosed one leaves the expansion open: in `${x[{]}` the `}` closes the `{`, and Zsh reports `closing brace expected`.
The parser fork closed the expansion at the `}` after the `]`, which accepted `${x[{]}` and rejected `${x[{}]}`.

For Zsh, the subscript of an unquoted `${...}` now counts braces.
While a `{` is open, blanks and operators are text, and a `{` still open at the `]` is an error at that `{`.
Subscript flags read their argument as raw text; that reader counts braces too, so the `,` in `${x[(r){a,b}]}` is text, while a nested `${...}` and quoted braces keep their own reading.
A short `$x[...]` and a double-quoted `"${x[...]}"` count no braces against the expansion and are unchanged.
Every change is behind `LangZsh`.

## Known false rejections

Zsh also accepts a `{` left open at the `]` when later text of the same word closes it: in `x=${x[{]}}` the first `}` after the `]` closes the `{` and the second closes the expansion, and in `x=${y[{]}]}` the `]}` does.
The parser has no place for text that closes a brace of an expansion from outside it, so it rejects these.
The maintainer chose this over a larger change to the syntax tree.
Base rejected the command-word forms, as in `print ${x[{]}}`, and accepted the assignment forms by accident.
13 grid rows of the assignment form are now rejected: `x=${x[S]}}` and `x=${x[S]}]}` for each of `{`, `a{`, `{a`, `1,{` and `(r){` as `S`, plus `x=${y[{]}]}`, `x=${x[${y[{]}]}]}` and `print $x[${y[{]}]}]`.

## Rows

All are top-level probes unless marked.

| Row | Source                   | Native | Base   | Fixed  |
| --- | ------------------------ | ------ | ------ | ------ |
| 01  | `print ${x[{]}`          | reject | accept | reject |
| 02  | `print ${x[a{]}`         | reject | accept | reject |
| 03  | `x=${x[{]}`              | reject | accept | reject |
| 04  | `[[ ${x[{]} == a ]]`     | reject | accept | reject |
| 05  | `print ${x[1,{]}`        | reject | accept | reject |
| 06  | `print ${x[$y{]}`        | reject | accept | reject |
| 07  | `print ${x[(r){]}`       | reject | accept | reject |
| 08  | `print ${x[(r)a,{]}`     | reject | accept | reject |
| 09  | `print ${x[{}]}`         | accept | reject | accept |
| 10  | `print ${x[{a}]}`        | accept | reject | accept |
| 11  | `print ${x[a{b}c]}`      | accept | reject | accept |
| 12  | `print ${x[{a b}]}`      | accept | reject | accept |
| 13  | `print ${x[{a;b}]}`      | accept | reject | accept |
| 14  | `print ${x[{$y}]}`       | accept | reject | accept |
| 15  | `print ${x[(r){a,b}]}`   | accept | reject | accept |
| 16  | `print ${x[(r)a,{b}]}`   | accept | reject | accept |
| 17  | `print ${x[(r){a}]}`     | accept | accept | accept |
| 18  | `print ${m[(i)${Y[a]}]}` | accept | accept | accept |
| 19  | `print ${x[(r)'{']}`     | accept | accept | accept |
| 20  | `print $x[{]`            | accept | accept | accept |
| 21  | `print "${x[{]}"`        | accept | accept | accept |
| 22  | `x=${x[{]}}`             | accept | accept | reject |
| 23  | `print ${x[{]}}`         | accept | reject | reject |
| 24  | `print $x[{a}]`          | accept | reject | reject |

Row 22 is one of the known false rejections above; row 23 is its command-word form, already rejected by base.
Row 24 is a short subscript, which this change leaves unchanged.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 102 files compared, 101 unchanged, 1 `FIXED` (the new `ok-subscript-balanced-braces.zsh`).
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged. A first draft of the flag-argument count regressed files with a nested `${...}` in a flag argument, as in `${tmp[(r)${start%%[^a-zA-Z0-9_-]#}]}`; the final change keeps a nested expansion's braces out of the count.
- Generated grid: 1477 rows (70 subscript shapes in 16 contexts, 7 open-brace shapes with 12 trailing texts in 5 contexts, and 27 nesting probes): 204 fixed false accepts and 235 fixed rejections.
  The other changes are the 13 known false rejections above and 27 runtime-tier acceptances: sources such as `print ${x[{a[b}]}` and `print ${x[${y[{a}]}]}`, which `zsh -f -n` rejects at the top level only as expansion errors (`bad substitution`, `invalid subscript`) and accepts in a function body.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 22 of 23 killed. The survivor drops the nested-expansion guard on the flag-argument error, which only changes the message on sources already rejected for another reason; it changes no verdict over 1557 rows.
- Corpus fixture: passes `zsh -f -n` and runs under `zsh -f`, printing the keys Zsh reads.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
