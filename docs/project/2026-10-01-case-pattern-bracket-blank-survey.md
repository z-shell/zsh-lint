# 2026-10-01 case pattern bracket blank survey

Issue: [#483](https://github.com/z-shell/zsh-lint/issues/483).
Base: `37a3f48` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.
Manual: [Complex Commands](https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands) and [Glob Operators](https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators).

## Change

Zsh lexes a case item that begins with `(` as one word up to the matching `)` (`gettokstr` in `Src/lex.c` ends a word at a blank only outside parentheses), and `par_case` in `Src/parse.c` then strips only the blanks next to `(`, `|` and `)` at the group's outer level.
A blank anywhere else in the group is pattern text, so in `(a[ b]*)` the blank is a member of the bracket expression.
The fork's lexer ended the word at the blank and reported `case patterns must be separated with |`.

The parser now sets a Zsh-only flag while it reads the first word of a case item that begins with `(` or `((`, up to the `)` that closes the item's group, as the #482 flag does.
While it is set, the lexer keeps a blank as literal text when the literal read so far ends inside a bracket expression: a `[` whose `]` has not come yet, where a `]` right after the `[` (or after `!` or `^`) is a member and a `[:class:]` is skipped whole.
When an expansion or quote inside the brackets ends the literal, as in `(a[$x b])`, the lexer records that the bracket is still open and keeps the next blank too, unless a `|` came between them.
`preNested` clears both flags and `postNested` restores them, so a command substitution in the pattern reads its own words as before.
The flag clears when the item's leading group closes, so `(x)y[ z])` stays rejected, as Zsh rejects it.
The printer writes a case item's optional `(` when a pattern holds a blank, since without it Zsh would end the word there; `TestStructuralOracle` caught the missing parenthesis.

Every other blank in the group still ends the word: `(a b)`, `(a[x] b)` and `( #i)x)` are [#578](https://github.com/z-shell/zsh-lint/issues/578).

## Rows

Each pattern is the item of `case $x in <pattern> print a ;; esac`, at the top level.

| Row | Pattern              | Native | Base   | Fixed  |
| --- | -------------------- | ------ | ------ | ------ |
| 1   | `(a[ b]*)`           | accept | reject | accept |
| 2   | `(include[ $TAB]*)`  | accept | reject | accept |
| 3   | `([ ])`              | accept | reject | accept |
| 4   | `(a\|[ ])`           | accept | reject | accept |
| 5   | `([!] ])`            | accept | reject | accept |
| 6   | `([[:space:] ])`     | accept | reject | accept |
| 7   | `(a[$x b])`          | accept | reject | accept |
| 8   | `(a[${x[1 + 2]} b])` | accept | reject | accept |
| 9   | `(a[ (b) ])`         | accept | reject | accept |
| 10  | `(a[x b)`            | accept | reject | accept |
| 11  | `a[ b]*)`            | reject | reject | reject |
| 12  | `(x)y[ z])`          | reject | reject | reject |
| 13  | `(a[ ;])`            | reject | reject | reject |
| 14  | `(a[$x) b])`         | reject | reject | reject |
| 15  | `(a[x] b)`           | accept | reject | reject |
| 16  | `(a[$x\| b])`        | accept | accept | accept |

Row 10 holds a `[` that never closes; Zsh reads it as a word with an unmatched bracket, valid to the parser and `bad pattern` only when the item is tried.
Row 15 is a blank outside the brackets, #578.
Row 16 parses on both builds, but as the two patterns `a[$x` and `b]` where Zsh reads one word; the blank after a `|` is one `par_case` strips at the outer level, and inside the brackets Zsh keeps it, so the tree is wrong on both builds.
It needs the whole-group reading #578 asks for and is left as it was.

## Verification

- `.github/scripts/verify-parser-change.sh` with a bodies grid, a rows grid and a file list: build, vet, tests, the fork's tests, `golangci-lint` v2.12.2, compare and both grids pass.
- Corpus: 108 files, 107 unchanged, `ok-case-pattern-bracket-blank.zsh` fixed.
- Workspace files (1150: F-Sy-H, Zi, `src`, and the Zsh `Completion` tree vendored in zpmod): 1149 unchanged, `F-Sy-H/functions/_fsh_make_targets` fixed, no regression and no false accept.
- Row grid (3528 patterns, each in five contexts, 17640 files), judged with `zsh -f -n` against both builds: 8030 fixed, 0 regressed, 0 false accepts introduced.
  60 rows are false accepts on both builds: a short subscript whose text begins with a blank, as in `$x[ b])` with no opening `(`, which Zsh rejects with `invalid subscript`; filed separately.
- Hand mutants, because `mutation.sh` does not mutate the fork: 26, 24 killed, 2 lived (listed in the pull request).
  Both survivors drop a reset of the carried bracket state (outside the opened group, and across a nested substitution); `zshCaseBracketBlank` reads that state only while the group's flag is set, so a stale value is inert, and the 17640-row grid gives the same verdict on every row with either reset removed.
  The resets stay so that no reader inherits a stale value.
