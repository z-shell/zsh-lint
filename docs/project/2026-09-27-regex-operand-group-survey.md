# 2026-09-27 regex operand group survey

Issue: [#517](https://github.com/z-shell/zsh-lint/issues/517).
Base: `cc5beb10` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Conditional Expressions](https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html); the rule is `gettokstr` in `Src/lex.c`, at the `zsh-5.9.2` release, which lexes an `=~` operand as it lexes any other word.

## Change

The parser fork reads an `=~` operand with its own lexer, `advanceLitRe`, which kept `;`, `&`, `<` and `>` inside an open group.
Zsh lexes the operand as an ordinary word, so, as #511 established for other words, these characters end it inside the group, unless `<(` or `>(` starts a process substitution or `<m-n>` is a numeric glob.
For Zsh only, `advanceLitRe` now ends the word there, reads a numeric glob longer than the read buffer as #511 does, and records the outermost open `(` so that the `)` left over after `&&` is reported where it stands, naming that `(`.
Bash keeps its own regex reading.

## Rows

All are top-level probes unless marked.

| Row | Source                       | Native | Base   | Fixed  |
| --- | ---------------------------- | ------ | ------ | ------ |
| 01  | `[[ x =~ a(b;c) ]]`          | reject | accept | reject |
| 02  | `[[ x =~ a(b&c) ]]`          | reject | accept | reject |
| 03  | `[[ x =~ a(b<c) ]]`          | reject | accept | reject |
| 04  | `[[ x =~ a(b>c) ]]`          | reject | accept | reject |
| 05  | `[[ x =~ (b;c) ]]`           | reject | accept | reject |
| 06  | `[[ x =~ a(b&&c) ]]`         | reject | accept | reject |
| 07  | `[[ x =~ a(<x>) ]]`          | reject | accept | reject |
| 08  | `[[ x =~ a(<1-2) ]]`         | reject | accept | reject |
| 09  | `[[ x =~ a(<(y);c) ]]`       | reject | accept | reject |
| 10  | `[[ y && x =~ a(b;c) ]]`     | reject | accept | reject |
| 11  | `f() { [[ x =~ a(b;c) ]]; }` | reject | accept | reject |
| 12  | `[[ x =~ a(b&&c ]]`          | accept | reject | accept |
| 13  | `[[ x =~ a(b && y ]]`        | accept | reject | accept |
| 14  | `[[ x =~ a(b\|c) ]]`         | accept | accept | accept |
| 15  | `[[ x =~ a(b\|\|c) ]]`       | accept | accept | accept |
| 16  | `[[ x =~ a(b c) ]]`          | accept | accept | accept |
| 17  | `[[ x =~ a(<1-2>) ]]`        | accept | accept | accept |
| 18  | `[[ x =~ a(<(y)) ]]`         | accept | accept | accept |
| 19  | `[[ x =~ a("b;c") ]]`        | accept | accept | accept |
| 20  | `[[ x =~ a($(b;c)) ]]`       | accept | accept | accept |
| 21  | `[[ x =~ a(() && y ]]`       | reject | reject | accept |
| 22  | `[[ x =~ a(()) ]]`           | reject | accept | accept |

Rows 12 and 13 are valid: the word ends at the `&&` with its group open, and the rest is a connective and an operand.
Row 21 is newly accepted, but it belongs to a false accept that base already has for every `[[ ]]` operand, row 22: Zsh rejects an empty `()` inside a group (`parse error near ()`), and the fork does not.
Base rejected row 21 only because its regex reader kept the `&&` inside the group and reached the end of input.
It is filed as [#522](https://github.com/z-shell/zsh-lint/issues/522) rather than fixed here, because the same gap covers `==` and `-n` operands, which this change does not touch.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 100 files, all unchanged.
- Workspace: the same comparison over 333 Zsh files under `repos/` of the Z-Shell workspace: all unchanged.
- Vendored Zsh: 1378 files from `zpmod`'s vendored `Completion/`, `Functions/` and `Test/` trees: all unchanged.
- Generated grid: 4115 rows (33 group bodies, 5 prefixes, 5 suffixes, 5 contexts): 805 fixed false accepts and 100 fixed rejections; the only other changes are the 5 rows of row 21's shape, one per context.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 10 of 11 killed. The survivor lets a malformed long numeric glob continue the word after its error; the parser already stops at the first error, so it is equivalent.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
