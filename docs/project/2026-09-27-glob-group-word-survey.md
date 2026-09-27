# 2026-09-27 glob group word survey

Issue: [#511](https://github.com/z-shell/zsh-lint/issues/511).
Base: `8b89d45f2a5b5ddaf59de6fdd14c3a334fdf1f68` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Glob Operators](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators); the lexer rule is `gettokstr` in `Src/lex.c` at the `zsh-5.9.2` release.

## Change

The parser fork now lexes a glob group in any word as Zsh does.
A `;`, `&`, or a `<` or `>` that starts neither a process substitution nor a numeric glob `<m-n>` ends the word inside the group.
The group stays open and the byte starts the next token, so `print a(b;c)` reads as `print a(b`, `;` and `c)`, and the stray `)` is the error Zsh reports.
A `(` inside a parameter expansion in the group opens nothing, following `in_brace_param` in `gettokstr`.
In `[[ ]]`, a `)` left over by such a word is reported where it stands instead of at the `[[`.
All of it sits behind `LangZsh`, in the glob-group reader, so other dialects are unchanged.
The #484 condition-operand lexing (`zshCondGroupRune`) is unchanged and still takes precedence for `-NAME` operands.
A bare `(` still does not nest in an ordinary word; that is #397.
No compatibility adapter was added.

Two existing tests of the nested conditional pattern adapter asserted that the raw parse failed with the adapter's gating error.
The fork now rejects those sources earlier, at the operator, so the precondition asserts only that the raw parse fails; every row and its `Parse()` rejection are unchanged.
A #484 printer round-trip row, `[[ -foo a ${x:-$(print a(b;c))} ]]`, moved to a #511 test that checks the tree without printing, because the printer writes the `;` inside the substitution as a newline.

## Rows

Rows 01-14 are the issue's rows; rows 15-33 are additional probes.
All are top-level probes.

| Row | Source                        | Native | Base   | Fixed  |
| --- | ----------------------------- | ------ | ------ | ------ |
| 01  | `[[ x == a(b;c) ]]`           | reject | accept | reject |
| 02  | `[[ x == a(b&c) ]]`           | reject | accept | reject |
| 03  | `[[ x == a(b>c) ]]`           | reject | accept | reject |
| 04  | `[[ x == a(b<c) ]]`           | reject | accept | reject |
| 05  | `print a(b;c)`                | reject | accept | reject |
| 06  | `print a(b&c)`                | reject | accept | reject |
| 07  | `print a(b>c)`                | reject | accept | reject |
| 08  | `print a(<x>)`                | reject | accept | reject |
| 09  | `x=a(b;c)`                    | reject | accept | reject |
| 10  | `[[ x == a(b\|c) ]]`          | accept | accept | accept |
| 11  | `print a(<1-2>)`              | accept | accept | accept |
| 12  | `print a(b\|c)`               | accept | accept | accept |
| 13  | `print a(${x:-(})`            | accept | reject | accept |
| 14  | `[[ x == a(${x:-(}) ]]`       | accept | reject | reject |
| 15  | `[[ x == a(b&&c) ]]`          | reject | accept | reject |
| 16  | `[[ x == (b;c) ]]`            | reject | accept | reject |
| 17  | `case x in a(b;c)) ;; esac`   | reject | accept | reject |
| 18  | `for i in a(b;c); do :; done` | reject | accept | reject |
| 19  | `f=( a(b;c) )`                | reject | accept | reject |
| 20  | `print *(e:x;y:)`             | reject | accept | reject |
| 21  | `print a(<1-2)`               | reject | accept | reject |
| 22  | `print a(<1>)`                | reject | accept | reject |
| 23  | `print ${x:-$(print a(b;c))}` | accept | accept | accept |
| 24  | `print a(<(x))`               | accept | reject | accept |
| 25  | `print a(b<(x))`              | accept | reject | accept |
| 26  | `print a("${x:-(}")`          | accept | reject | accept |
| 27  | `(print a(b;c)`               | accept | reject | accept |
| 28  | `[[ x == a(b&&c ]]`           | accept | reject | accept |
| 29  | `[[ x =~ a(b;c) ]]`           | reject | accept | accept |
| 30  | `print a(b(c)d)`              | accept | reject | reject |
| 31  | `print a(${x#(})`             | reject | reject | accept |
| 32  | `print a(${x/(/y})`           | reject | reject | accept |
| 33  | `print a(${x[(i)(]})`         | reject | reject | accept |

Row 14 still fails: the fork parses it, but the pre-scan of the nested conditional pattern adapter (#120) counts the `(` inside the unquoted `${...}`.
It is filed as [#513](https://github.com/z-shell/zsh-lint/issues/513).
Row 29 is unchanged: the `=~` operand goes through the fork's separate regular-expression lexer, which is outside this change.
Row 30 is the nested-group gap, #397.

Rows 31-33 are runtime-tier.
`zsh -f -n` rejects each at the top level with an expansion error (`bad pattern: (` for rows 31 and 32, `invalid subscript` for row 33), and accepts the same text inside a function body, where only the lexer runs (#287).
Base rejected them for a lexer reason that is wrong for Zsh: it counted the `(` inside the parameter expansion as a group opener.
The fixed parser reads them as Zsh's lexer does, so their remaining native error is an expansion error that zsh-lint does not model.

Row 23 is valid Zsh because the `)` after `c` closes the command substitution; the substitution runs `print a(b` and then `c`, and the last `)` is literal text.
Zsh 5.9.2 prints `bad pattern: a(b` followed by `)` when it runs.

A numeric glob is recognized by looking ahead in the parser's 1 KiB read buffer.
A review of the first head found that one longer than the buffer was read as a bare `<`, so `print a(<` followed by 1022 digits and `-2>)` was rejected although it is valid Zsh.
A numeric glob that fills the buffer is now read to its end instead: any valid length parses, and a shape that fails past the buffer is reported at the failing byte, as in `` a numeric glob cannot contain `)` ``.
Every rejected shape in that path is also a native parse error, including `<` followed by more than 1 KiB of digits and `>`, which base accepted.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 99 files compared, 99 unchanged.
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: 333 unchanged; the known 2 gaps and 1 false accept are pre-existing and untouched.
- Probe grid: `zsh-lint-probe` over 25 bodies in every scanner context, 650 files: 77 `FIXED`, 278 `REJECTED` (the intended false-accept fixes), 295 unchanged, no `REGRESSED` and no `FALSE-ACCEPT`.
- Crash probe: 1040 rows with each follow-up token after an open group, and 360 rows with numeric globs of 1015 to 2100 digits in five contexts, under `timeout 5`: no panic and no hang.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 24 of 25 killed.
  The survivor, which lets a `>` start a numeric glob check, is equivalent: a `>` never passes that check with a different verdict, since `>` followed by digits and `-` and `>` is a redirect that Zsh and the fork both reject.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
