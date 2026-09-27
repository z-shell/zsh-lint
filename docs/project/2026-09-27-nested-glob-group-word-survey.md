# 2026-09-27 nested glob group word survey

Issue: [#397](https://github.com/z-shell/zsh-lint/issues/397).
Base: `c3257037` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid, and a run under `timeout 3 zsh -f` where `-n` cannot decide.
Manual: [Glob Operators](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators).
Source: `gettokstr` in `Src/lex.c` (`pct`, `LX2_INPAR`, `LX2_OUTPAR`).

## Change

The glob group reader for command and array words (`zshWordGroupRune`, since #511) closed a group at the first unquoted `)` outside a substitution.
A bare `(` inside the group did not nest, so in `print a(b(c)d)` the inner `)` closed the group and the remaining `d)` was lexed as new tokens, a parse error.
Zsh counts every bare `(` in a word (`pct` in `gettokstr`) and only its matching `)` closes it.

A bare `(` inside a word's glob group now nests, so only its matching `)` closes it.
Inside the nested group the rules of the outer one apply: quotes and substitutions nest, `;`, `&` and a bare `<` or `>` end the word, and `()` is a token of its own (#522).
This fixes the issue's rows, including the `z-shell/zi` line `lib/zsh/install.zsh:1783`, and rejects the false accept `f=( (a|(b|c) )`, whose nested group is never closed.

`[[ ]]` operands are left as before: their nested groups still go to the conditional-pattern adapter (#202), which reads them from the parser's error.

A nested group left open to the end of input is now rejected, as a single open group already was.
`zsh -f -n` accepts such a word, because it does not match the pattern.
In a command word it fails when run with `bad pattern` (row 22).
In a plain assignment it runs cleanly (row 23), because assignments do not glob. That is the class `x=a(b` already had on base (row 24), and the maintainer chose to record it rather than widen this change.

## Rows

All are top-level probes. `\n` is a newline.

| Row | Source                                                         | Native | Base   | Fixed  |
| --- | -------------------------------------------------------------- | ------ | ------ | ------ |
| 01  | `f=( (a\|(b\|c)) )`                                            | accept | reject | accept |
| 02  | `f=( (a\|.(b\|c)) )`                                           | accept | reject | accept |
| 03  | `f=( x(a\|(b\|c)) )`                                           | accept | reject | accept |
| 04  | `print (a\|.(b\|c))`                                           | accept | reject | accept |
| 05  | `f=( *~(a\|.(b\|c))/* )`                                       | accept | reject | accept |
| 06  | `files=( (#i)**/*.(zip\|rar)~(*/*\|.(_backup\|git))/*(-.DN) )` | accept | reject | accept |
| 07  | `print a(b(c)d)`                                               | accept | reject | accept |
| 08  | `f=( a(b(c)d) )`                                               | accept | reject | accept |
| 09  | `print a(b(c)\nd)`                                             | accept | reject | accept |
| 10  | `case x in a(b(c))) ;; esac`                                   | accept | reject | accept |
| 11  | `x=a(b(c))`                                                    | accept | reject | accept |
| 12  | `print a(b(${x:-)}))`                                          | accept | reject | accept |
| 13  | `f=( (a\|b) )`                                                 | accept | accept | accept |
| 14  | `f=( *.(zip\|rar)~(*/*)/*(-.DN) )`                             | accept | accept | accept |
| 15  | `print (#i)*.zip`                                              | accept | accept | accept |
| 16  | `[[ x == a(b(c)) ]]`                                           | accept | accept | accept |
| 17  | `print a(b<(c))`                                               | accept | accept | accept |
| 18  | `f=( (a\|(b\|c) )`                                             | reject | accept | reject |
| 19  | `print a(b(c)))`                                               | reject | reject | reject |
| 20  | `print a(b(c;d))`                                              | reject | reject | reject |
| 21  | `print a(b(()))`                                               | reject | reject | reject |
| 22  | `print a(b(c)`                                                 | accept | accept | reject |
| 23  | `x=a(b(c)`                                                     | accept | accept | reject |
| 24  | `x=a(b`                                                        | accept | reject | reject |
| 25  | `print ((a\|b))`                                               | accept | reject | reject |

Rows 13 to 17 are the issue's keep-verdict rows and parse to the same trees.
Row 25 is a separate gap left as it was: a word that starts with `((` is read as an arithmetic command.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, 102 unchanged and the new `ok-nested-glob-group-word.zsh` fixed. The new file passes `zsh -f -n`, runs under `zsh -f` printing every line, and fails on base.
- Workspace: the same comparison over 311 Zsh files under `repos/` of the Z-Shell workspace: 310 unchanged, `zi/lib/zsh/install.zsh` fixed. Over 1487 files of the corpus audit tree: 1478 unchanged, 9 fixed.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: 1371 unchanged, 7 fixed (`_btrfs`, `_cryptsetup`, `_debfoster`, `_modutils`, `_twidge`, `_yum`, `_zftp`).
- Generated grid: 1840 rows (26 group bodies, 9 group shapes, 8 contexts): 539 fixed false rejects, 70 fixed false accepts and no introduced false accept.
  42 rows are newly rejected, all with a nested group open to the end of input (rows 22 and 23): 31 fail when run (26 with `bad pattern`), and the 11 that run clean are plain assignments.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 6 of 6 killed. A first-round survivor showed a `[[ ]]` guard in the caller was redundant, since a `[[ ]]` operand never pushes a nested group; the guard was removed.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
