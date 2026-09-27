# 2026-09-27 group substitution body survey

Issue: [#519](https://github.com/z-shell/zsh-lint/issues/519).
Base: `bc6d0f0a` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Command Substitution](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution) and [Glob Operators](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators).

## Change

A glob group reader in the parser fork steps over a command substitution, backquoted command or process substitution to find the group's `)`, and keeps its bytes as text of the group's literal.
Nothing parsed those bytes as commands again, so `print a($(done))` was accepted while `print $(done)` was rejected.
Zsh parses the body in both places.

The reader now follows each such substitution.
When the outermost one closes, its body is parsed on its own, and an error in it is reported at its place in the source.
A backslash before a newline, which the reader drops, is put back first; inside backquotes, a backslash before `$`, `` ` `` or `\` is removed first, as the backquoted command reads it, and error positions count the removed bytes.
A `$((` body that may be arithmetic is left as before.

The check applies in every context, matching what the parser already does for a substitution outside a group.
In an assignment, a `[[ ]]` operand, a case pattern or a function body, `zsh -f -n` does not parse a substitution's body until it runs, so such a row passes `zsh -f -n` and is rejected here.
The maintainer chose this over a check limited to command words, because the parser already rejects the same substitution in those contexts outside a group: `x=$(if true)` and ``[[ x == `done` ]]`` are rejected on base.

## Rows

All are top-level probes unless marked.

| Row | Source                          | Native | Base   | Fixed  |
| --- | ------------------------------- | ------ | ------ | ------ |
| 01  | `print a($(done))`              | reject | accept | reject |
| 02  | `print a(b\|$(done))`           | reject | accept | reject |
| 03  | `print a(${x:-$(done)})`        | reject | accept | reject |
| 04  | `print a(<(done))`              | reject | accept | reject |
| 05  | `print a("$(done)")`            | reject | accept | reject |
| 06  | `print a($(echo $(done)))`      | reject | accept | reject |
| 07  | `print a(b)($(done))`           | reject | accept | reject |
| 08  | `[[ x == a($(done)) ]]`         | reject | accept | reject |
| 09  | `[[ x == (a\|(b\|$(done))) ]]`  | reject | accept | reject |
| 10  | `case x in a($(done))) ;; esac` | reject | accept | reject |
| 11  | ``print a(`done`)``             | reject | accept | reject |
| 12  | ``print a(`echo \$(done)`)``    | reject | accept | reject |
| 13  | `print a($(true))`              | accept | accept | accept |
| 14  | `print a($(echo ')'))`          | accept | accept | accept |
| 15  | `print a($((1+2)))`             | accept | accept | accept |
| 16  | `print a('$(done)')`            | accept | accept | accept |
| 17  | ```print a(`echo \`true\``)```  | accept | accept | accept |
| 18  | ``x=a(`done`)``                 | accept | accept | reject |
| 19  | `` x=`done` ``                  | accept | reject | reject |
| 20  | `print a($( (1<<2) ))`          | accept | accept | reject |
| 21  | `print $( (1<<2) )`             | accept | reject | reject |
| 22  | `print ${x[(r)$(done)]}`        | reject | accept | accept |

Row 18 is one of the rows the maintainer's choice rejects: `zsh -f -n` accepts it, and running it reports a parse error in the substitution.
Row 19 is the same substitution outside a group, which base already rejects.
Row 20 is valid Zsh rejected for a different reason: the parser reads `(1<<2)` as a here-document, as it does outside a group (row 21), a pre-existing gap.
Row 22 is a subscript-flag argument, a separate reader, filed as #526.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 102 files, all unchanged.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace and 1487 files of the corpus audit tree: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 3154 rows (38 bodies, 7 substitution forms, 12 contexts): 768 fixed false accepts and no introduced false accept.
  225 rows are newly rejected, all under the maintainer's choice above: 213 fail when run (165 with a parse error in the substitution), and the 12 that run clean all hold the `(1<<2)` body of row 20.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 18 of 19 killed. The survivor also checks an empty body, which parses without error; the comment on that condition says so.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
