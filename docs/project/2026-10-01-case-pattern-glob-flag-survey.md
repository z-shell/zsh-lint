# 2026-10-01 case pattern globbing flag survey

Issue: [#482](https://github.com/z-shell/zsh-lint/issues/482).
Base: `6fdf7ec` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.
Manual: [Globbing Flags](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Globbing-Flags) and [Complex Commands](https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands).

## Change

Zsh lexes a case item that begins with `(` as one word and only afterwards decides whether the `(` was the optional opener (`par_case` in `Src/parse.c`).
Inside that word a `#` is pattern text, so `(#i)x)` is the globbing flag `(#i)` followed by `x`.
The fork's lexer read a `#` that began a token as a comment, so after the `(` token it took `#i)x) print a ;;` for a comment and reported `case patterns must consist of words`.
The same happened to a `#` glued to a `|` inside that group, `(a|#i)x)`, and to a `#` glued to the `)` of a leading group, the glob operator in `(x)#)`.

The parser now sets a Zsh-only flag while it reads the first word of a case item that begins with `(` or `((`, up to the `)` that closes the item's group, and the lexer treats a `#` glued to a `(`, `|` or `)` token as pattern text while the flag is set.
`preNested` clears the flag and `postNested` restores it, so a command substitution inside the pattern keeps its own comments.
A `#` after a blank, after the `|` of an item with no opener or whose leading group has closed, and anything after the item's closing `)` keep their meaning as a comment.

## Rows

Each pattern is the item of `case $x in <pattern> print a ;; esac`, at the top level.

| Row | Pattern                           | Native | Base   | Fixed  |
| --- | --------------------------------- | ------ | ------ | ------ |
| 1   | `(#i)x)`                          | accept | reject | accept |
| 2   | `(#b)(--adj)(=(-\|+\|)[0-9]#\|))` | accept | reject | accept |
| 3   | `(#i)x\|y)`                       | accept | reject | accept |
| 4   | `(#))`                            | accept | reject | accept |
| 5   | `(a\|#i)x)`                       | accept | reject | accept |
| 6   | `(x)#)`                           | accept | reject | accept |
| 7   | `((x)\|#i))`                      | accept | reject | accept |
| 8   | `($(print a)\|#i))`               | accept | reject | accept |
| 9   | `(#i)x)#)`                        | reject | reject | reject |
| 10  | `((#i)x))`                        | accept | accept | accept |
| 11  | `x\|#i)`                          | reject | reject | reject |
| 12  | `(x)\|#i)`                        | reject | reject | reject |
| 13  | `(#i)x\|#j)`                      | reject | reject | reject |
| 14  | `(a\|$(print x\|#c))x)`           | reject | reject | reject |
| 15  | `(#i)x print a ;;`                | reject | reject | reject |
| 16  | `(a\| #i)x)`                      | accept | reject | reject |
| 17  | `( #i)x)`                         | accept | reject | reject |
| 18  | `(a #b))`                         | accept | reject | reject |

Row 9 is rejected because the `#` after the item's `)` starts a comment that swallows `print a ;; esac` on the same line.
With `esac` on the next line Zsh accepts it, reading the rest of the line as a comment and the arm as `((#i)x)` with an empty body, and so does the fixed build; `TestCasePatternHashStartsComment` pins that tree and the comment.
Rows 9 and 11 to 15 keep the comment reading: in each the `#` follows a blank, a `|` outside the item's group, or the item's closing `)`, or sits in a command substitution.
Rows 16 to 18 hold a blank inside the item's leading group, which Zsh's whole-word lexing keeps and the fork's tokenizing does not; that is a separate gap, [#578](https://github.com/z-shell/zsh-lint/issues/578), and stays rejected on both builds.
`(a|#i)b)` passes `zsh -f -n` but is `bad pattern` when it runs, since a globbing flag must open a group or pattern; it is accepted like any other pattern text.

## Verification

- `.github/scripts/verify-parser-change.sh` with a bodies grid, a rows grid and a file list: build, vet, tests, the fork's tests, `golangci-lint` v2.12.2, compare, both grids and the parse-count trace all pass.
- Corpus: 108 files, 106 unchanged, `ok-case-pattern-glob-flag.zsh` fixed.
- F-Sy-H and Zi (157 files from the workspace checkouts): 156 unchanged, `F-Sy-H/chroma/_fsh_chroma_nice` fixed, no regression and no false accept.
- Probe grid (49 patterns in every `zsh-lint-probe` context, 1274 files): 410 unchanged, 760 fixed, 104 moved, 0 regressed, 0 false accepts.
  The moved rows are the five native-invalid patterns, whose first error now sits later in the item.
- Row grid (49 patterns at the top level, in a function, in `$( )` and after another arm, 196 files): 63 unchanged, 113 fixed, 20 moved, 0 regressed, 0 false accepts.
- Parse cost: one parse for the new fixture on both builds.
- Hand mutants (12), because `mutation.sh` does not mutate the fork: 11 killed, 1 lived.
  The survivor removes the `!p.spaced` condition; it makes the fork accept five more rows, every one valid Zsh (`( #i)x)`, `(a| #i)x)`, `(a | #c)`, `( #)`, `(a| #)`), and no invalid row in the grids.
  A spaced `#` in the group is part of [#578](https://github.com/z-shell/zsh-lint/issues/578): the fork ends the word at the blank before it, so the condition keeps this change to the glued `#` rather than closing part of #578, and no test can pin it without asserting that valid Zsh stays rejected.
