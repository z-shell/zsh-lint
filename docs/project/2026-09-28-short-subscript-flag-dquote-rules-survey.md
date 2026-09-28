# 2026-09-28 short subscript flag dquote rules survey

Issue: [#540](https://github.com/z-shell/zsh-lint/issues/540).
Base: `feefae12` (`origin/main`), and `f536c0c0` from before #538, both built with `go build` into scratch binaries.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Where `-n` accepts, a run under `timeout 3 zsh -f` with `$x`, `$y` and `$z` set to arrays decides, judged by stderr alone.
Manual: [Subscript Flags](https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags).
Source: `gettokstr`, `parse_subscript` and `dquote_parse` in `Src/lex.c` at the `zsh-5.9.2` tag.

## Change

[#538](https://github.com/z-shell/zsh-lint/issues/538) reports a `[` left open in a short `$x[...]` subscript flag argument when no later `]` of the word closes it, reading ahead with shell quoting.
Zsh reads the subscript when the word is expanded: `parse_subscript` runs `dquote_parse(']')` over the word's text, and its rules differ from the shell's:

- a `'` is text outside backquotes, so `${y:-'}]'}` closes at the `]` inside it (row 14);
- a `"` is text outside a nested `${...}`, where it opens a nested string (rows 7 and 15);
- a `$(...)` body is read as a command, so its brackets and parentheses are its own (rows 11 and 13);
- brackets inside a nested `${...}` do not count (rows 1 and 12), but those inside backquotes do (rows 3, 4 and 10);
- parentheses are counted like brackets, and a `)` or `]` that closes nothing is an error (rows 9 and 24);
- a backslash quotes a bracket, a parenthesis or a brace (row 17).

The read-ahead treated a double-quoted part of the word as opaque and a `'` as a quote, so it reported valid words (rows 11 to 15, regressions from #538) and missed invalid ones (rows 1 to 10).
It now finds the word's end as `gettokstr` does, where a blank or a `|` inside a glob group does not end the word but a `;` or a newline does, and a backslash quotes a backquote inside backquotes (row 25).
It then runs the `dquote_parse` count over the text up to that end, starting from the brackets and parentheses the argument left open.

Zsh then expands the subscript's text as a string, where a backquote is a command substitution again and a `'` quotes it.
When the text between the `]` and the subscript's closing `]` leaves a backquote open, as in rows 22 and 23, Zsh fails with `failed to find end of command substitution`; the fork now reports that backquote.
The base rejected these two only because its read-ahead missed the closing `]`, and the `dquote_parse` count alone accepts them.

The check applies in every context, as for the earlier subscript fixes.
In an assignment `zsh -f -n` does not check the subscript, so row 19 passes `zsh -f -n` and fails when run with `invalid subscript`; row 24 does the same in a command.
Both are runtime-tier: row 24 is `invalid-540-short-flag-runtime-parens.txt` in `runtimeTierInvalidFixtures`.

## Rows

All are top-level probes.

| Row | Source                                | Native       | Before #538 | Base   | Fixed  |
| --- | ------------------------------------- | ------------ | ----------- | ------ | ------ |
| 1   | `print "$x[(r)a[b]${y:-]}"`           | reject       | accept      | accept | reject |
| 2   | `print "$x[(r)a[b]$(echo ])"`         | reject       | accept      | accept | reject |
| 3   | ``print "$x[(r)a[b]`echo ]`"``        | reject       | accept      | accept | reject |
| 4   | ``print "$x[(r)a[b]`echo ]`]"``       | reject       | accept      | accept | reject |
| 5   | `print $x[(r)a[b]"${y:-]}"`           | reject       | accept      | accept | reject |
| 6   | `print $x[(r)a[b]"$(echo ])"`         | reject       | accept      | accept | reject |
| 7   | `print "$x[(r)a[b]${y:-"]"}"`         | reject       | accept      | accept | reject |
| 8   | `print "$x[(r)a[b]${y:-']'}"`         | reject       | accept      | accept | reject |
| 9   | `print "$x[(r)a[b](]"`                | reject       | accept      | accept | reject |
| 10  | ``print $x[(r)a[b]`echo ']'`]``       | reject       | accept      | accept | reject |
| 11  | `print "$x[(r)a[b]$(echo [)]"`        | accept       | accept      | reject | accept |
| 12  | `print "$x[(r)a[b]${y:-[}]"`          | accept       | accept      | reject | accept |
| 13  | `print "$x[(r)a[b]$(echo ")")]"`      | accept       | accept      | reject | accept |
| 14  | `print $x[(r)a[b]${y:-'}]'}`          | accept       | accept      | reject | accept |
| 15  | `print "$x[(r)a[b]${y:-"["}]"`        | accept       | accept      | reject | accept |
| 16  | `print "$x[(r)a[b]${y:-]}]"`          | accept       | accept      | accept | accept |
| 17  | ``print "$x[(r)a[b]`echo \]`]"``      | accept       | accept      | accept | accept |
| 18  | `print "$x[(r)a[b]$((1+(2)))]"`       | accept       | accept      | accept | accept |
| 19  | `x=$x[(r)a[b]"${y:-]}"`               | reject (run) | accept      | accept | reject |
| 20  | `print $x[(r)a[b]"]"`                 | accept       | reject      | reject | reject |
| 21  | `print $x[(r)a(]`                     | reject       | accept      | accept | accept |
| 22  | ``print $x[(r)a[b]'`'`echo "]"` ``    | reject       | accept      | reject | reject |
| 23  | ``print $x[(r)a[b]'`'${y:-`echo ]`}`` | reject       | accept      | reject | reject |
| 24  | `print $x[(r)a[(b]${y:-)}()]`         | reject (run) | accept      | accept | reject |
| 25  | ``print $x[(r)a[b]`echo \` x\``]``    | accept       | accept      | accept | accept |
| 26  | `print $x[(r)a[(b)]]`                 | accept       | accept      | accept | accept |

Rows 20 and 21 are known gaps left as they were.
In row 20 the flag-pattern adapter rejects a quoted `]` after the pattern.
In row 21 a `(` in the argument is left open at the only `]`; the read-ahead runs only when a `[` of the argument is open.
Rows 22 and 23 now fail at the backquote left open (columns 20 and 25) instead of at the `[`.
Row 25 was rejected by an earlier draft of this change, and row 26 guards the count of the argument's own parentheses.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated words: 23800 files (5 argument prefixes, one or two of 34 parts including nested expansions, substitutions, quotes and backquotes, and 4 suffixes, unquoted and in double quotes): 126 fixed, 4524 newly rejected invalid words, 2903 moved and no introduced false accept.
  The two regressed files are row 24 and `print $x[(r)a[(b]${y:-)}c()]`, which both fail when run with `invalid subscript`.
- Probe grid: the 41 source rows of the fork tests in the 26 contexts of `zsh-lint-probe`, 1066 files: 130 fixed, 154 newly rejected, 56 moved and no introduced false accept.
  The 284 regressed files are the 17 invalid rows in compound-command bodies, where `zsh -f -n` only lexes; each of the 17 fails when run inside a function.
- Parse cost (`-trace-parses`): corpus 624 parses before and after, grid 1430 before and 1376 after, adapter depth unchanged.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 29 of 36 killed.
  The 7 left are a `}` or a `"` inside backquotes within a nested `${...}`, a `]` with no `[` open while a parenthesis or backquote is, a `'` inside a double-quoted `${...}` body, the argument's own quoting when the word starts in double quotes, and a newline or a `$(...)` in the text scanned for an open backquote; none changed a verdict that native Zsh agrees with over 67040 generated words, the 23800 above and 43240 more with added quote and backquote parts.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
