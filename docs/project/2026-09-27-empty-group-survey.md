# 2026-09-27 empty group survey

Issue: [#522](https://github.com/z-shell/zsh-lint/issues/522).
Base: `13daada9` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Conditional Expressions](https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html); the rule is the `e == ')'` test at `LX2_INPAR` in `gettokstr`, `Src/lex.c`, at the `zsh-5.9.2` release.

## Change

Outside a parameter expansion, Zsh ends a word at a `(` whose next byte is `)`, and `()` becomes a token of its own.
Inside `[[ ]]` that token is an error, so `[[ x == a(()) ]]` is rejected.
The fork's three group readers treated `()` as an empty nested group, and the nested-pattern adapter masked the pair into an accepted source.

The fix applies the rule at each reader:

- the glob-group word reader (`zshWordGroupRune`, #511) ends the word, and the operator check reports the `(`;
- the condition-operand group reader (`zshCondGroupRune`, #484) reports `a condition glob group cannot contain` `()`, as it does for its other terminators;
- the `=~` reader (`advanceLitRe`, #517) ends the word at any depth. It reads the `(` of a process substitution itself, so that an empty `<()` is not taken for the token.
- The nested-pattern adapter marks a pattern holding `()` invalid instead of masking it.

Every change is behind `LangZsh`.
One existing fork test row changed with the maintainer's approval: `[[ a =~ ())` keeps its Bash message, and for Zsh now reports `1:9: not a valid test operator: (` at the `()`.

## Rows

All are top-level probes unless marked.

| Row | Source                      | Native | Base   | Fixed  |
| --- | --------------------------- | ------ | ------ | ------ |
| 01  | `[[ x == a(()) ]]`          | reject | accept | reject |
| 02  | `[[ x == (()) ]]`           | reject | accept | reject |
| 03  | `[[ x == a(b()c) ]]`        | reject | accept | reject |
| 04  | `[[ x == a(b\|()) ]]`       | reject | accept | reject |
| 05  | `[[ x == (a\|(b\|())) ]]`   | reject | accept | reject |
| 06  | `[[ x == a(""()) ]]`        | reject | accept | reject |
| 07  | `[[ a(() == x ]]`           | reject | accept | reject |
| 08  | `f() { [[ x == a(()) ]]; }` | reject | accept | reject |
| 09  | `[[ x =~ a(()) ]]`          | reject | accept | reject |
| 10  | `[[ x =~ a() ]]`            | reject | accept | reject |
| 11  | `[[ x =~ () ]]`             | reject | accept | reject |
| 12  | `[[ x =~ a(() && y ]]`      | reject | accept | reject |
| 13  | `[[ -n a(()) ]]`            | reject | accept | reject |
| 14  | `[[ a -foo b(()) ]]`        | reject | accept | reject |
| 15  | `[[ x == a(( )) ]]`         | accept | accept | accept |
| 16  | `[[ x == a((b)) ]]`         | accept | accept | accept |
| 17  | `[[ x == (a\|(b\|c)) ]]`    | accept | accept | accept |
| 18  | `[[ x == a('()') ]]`        | accept | accept | accept |
| 19  | `[[ x == a(\(\)) ]]`        | accept | accept | accept |
| 20  | `[[ x == a(${x:-()}) ]]`    | accept | accept | accept |
| 21  | `[[ x == a($(())) ]]`       | accept | accept | accept |
| 22  | `[[ x =~ a(<()) ]]`         | accept | accept | accept |
| 23  | `[[ -n a(<()) ]]`           | accept | accept | accept |

Row 12 was accepted by `13daada`, which merged #517; base before #517 rejected it only because its regex reader ran to the end of input.
Rows 20 and 21 stay valid: Zsh counts no parentheses inside a parameter expansion, and `$((` opens arithmetic.

``print a(`()`)`` remains a false accept, and `print a(( ))` a false rejection, part of the nested-group gap in #397; both are command words outside `[[ ]]`, which this change leaves unchanged.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 101 files, all unchanged.
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 4780 rows (the #517 regex grid, 34 word shapes in 20 contexts, and empty process substitutions): 178 fixed false accepts and no other change.
- Hand mutants of the fork and adapter change (the mutation script does not reach the fork module): 7 of 8 killed. The survivor drops the `return` after the condition reader's error, which is equivalent: the error already ends the loop, and 42 condition rows are unchanged under it. A ninth mutant removed a `zshBrokenGroup` assignment in the `=~` reader, which 42 rows showed unreachable, so the assignment was removed.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
