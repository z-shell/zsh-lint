# 2026-09-27 module condition operands survey

Issue: [#484](https://github.com/z-shell/zsh-lint/issues/484).
Base: `dd5dc1d8cb600c318a60c0352d9af2645d007b18` (`origin/main`, exported with `git archive`).
Oracle: Zsh 5.9.2, newline-terminated files checked with `timeout 5 zsh -f -n FILE`; empty stderr means valid.
Manual: [Conditional Expressions](https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html), also read from installed `zshmisc(1)`.
No network or GitHub operations were performed.

## Change

The parser fork accepts prefix module conditions with one or more operand words and infix module conditions with two operands.
A `ModuleTest` retains the name word, its position, infix placement, and operands, with walker, printer, and typed JSON support.
Known unary and binary tests retain their existing nodes; a lone unary name is a string test.
Both new entry points are gated on `LangZsh`.
No compatibility adapter was added.

The fork tests cover every requested row, additional boundary cases, exact rejection diagnostics, tree positions, traversal order, semantic printer round trips, typed JSON round trips, and explicit Bash/mksh/POSIX behavior.
Two old Zsh-only rejection expectations were removed after native confirmation of `[[ -n\na ]]` and `[[ (-e ) ]]`; their other dialect expectations remain.
The glued-brace rejection test still rejects its source, but now pins the missing closing bracket diagnostic after the extended operand list.

The requested production consumer search under `internal` and `cmd` returned no direct `TestExpr`, `UnaryTest`, `BinaryTest`, or `TestClause` consumers.
The analyzer and reference visitors use `syntax.Walk`.
An analyzer test runs all default rules without panic and separately proves that references in prefix, infix, and multi-operand conditions are visited.

## Rows

Rows 01-38 are the requested acceptance and rejection matrix; rows 39-48 are additional boundary probes.
All are top-level syntax probes.
The corpus fixture places the conditions in a never-called function because unloaded module names fail at runtime.
It passes both `timeout 5 zsh -f -n` and `timeout 5 zsh -f`.

| Row | Source                      | Native | Base   | Fixed  |
| --- | --------------------------- | ------ | ------ | ------ |
| 01  | `[[ -prefix - ]]`           | accept | reject | accept |
| 02  | `[[ -prefix -x ]]`          | accept | reject | accept |
| 03  | `[[ ! -prefix - ]]`         | accept | reject | accept |
| 04  | `[[ -prefix a ]]`           | accept | reject | accept |
| 05  | `[[ -after a b ]]`          | accept | reject | accept |
| 06  | `[[ -between a b c ]]`      | accept | reject | accept |
| 07  | `[[ -foo a b c d ]]`        | accept | reject | accept |
| 08  | `[[ -foo a ]] && print y`   | accept | reject | accept |
| 09  | `[[ a -foo b ]]`            | accept | reject | accept |
| 10  | `[[ -prefix - && -n x ]]`   | accept | reject | accept |
| 11  | `[[ -prefix - \|\| -z x ]]` | accept | reject | accept |
| 12  | `[[ ( -prefix - ) ]]`       | accept | reject | accept |
| 13  | `[[ -n a b ]]`              | accept | reject | accept |
| 14  | `[[ -z ]]`                  | accept | reject | accept |
| 15  | `[[ -n ]]`                  | accept | reject | accept |
| 16  | `[[ -f a b ]]`              | accept | reject | accept |
| 17  | `[[ -prefix a b c ]]`       | accept | reject | accept |
| 18  | `[[ -- a ]]`                | accept | reject | accept |
| 19  | `[[ -1 a ]]`                | accept | reject | accept |
| 20  | `[[ -a-b c ]]`              | accept | reject | accept |
| 21  | `[[ -prefix $x ]]`          | accept | reject | accept |
| 22  | `[[ -prefix "-" ]]`         | accept | reject | accept |
| 23  | `[[ -foo a -bar b ]]`       | accept | reject | accept |
| 24  | `[[ -foo a && b ]]`         | accept | reject | accept |
| 25  | `[[ -foo ]]`                | accept | accept | accept |
| 26  | `[[ -prefix ]]`             | accept | accept | accept |
| 27  | `[[ a -nt b ]]`             | accept | accept | accept |
| 28  | `[[ -z a ]]`                | accept | accept | accept |
| 29  | `[[ a == b ]]`              | accept | accept | accept |
| 30  | `[[ a -foo b c ]]`          | reject | reject | reject |
| 31  | `[[ a -eq b c ]]`           | reject | reject | reject |
| 32  | `[[ a == b c ]]`            | reject | reject | reject |
| 33  | `[[ - a ]]`                 | reject | reject | reject |
| 34  | `[[ -prefix ( ]]`           | reject | reject | reject |
| 35  | `[[ -prefix ) ]]`           | reject | reject | reject |
| 36  | `[[ -prefix ]] ]]`          | reject | reject | reject |
| 37  | `[[ -prefix < ]]`           | reject | reject | reject |
| 38  | `[[ a -foo ]]`              | reject | reject | reject |
| 39  | `[[ -n == x ]]`             | accept | reject | accept |
| 40  | `[[ -prefix a == b ]]`      | accept | reject | accept |
| 41  | `[[ -prefix a < b ]]`       | reject | reject | reject |
| 42  | `[[ -prefix a ! b ]]`       | accept | reject | accept |
| 43  | `[[ -prefix a ( b ) ]]`     | accept | reject | accept |
| 44  | `[[ -prefix a\nb ]]`        | accept | reject | accept |
| 45  | `[[ -prefix\na ]]`          | accept | reject | accept |
| 46  | `[[ -z && a ]]`             | accept | reject | accept |
| 47  | `[[ a -foo b -bar c ]]`     | reject | reject | reject |
| 48  | `[[ -prefix a > b ]]`       | reject | reject | reject |

## Comparisons

The corpus plus 58 probes contains 157 files: 110 unchanged, 37 fixed, 3 newly rejected invalid sources, and 7 moved diagnostics.
There are no regressions, introduced false accepts, known gaps, or known false accepts in that run.
The additional 414 operator/boundary probes report 186 unchanged, 153 fixed, 6 newly rejected invalid sources, and 69 moved diagnostics, with no regressions or introduced false accepts.

Four false accepts in the additional probes are unchanged from the base and outside this fix: `[[ - ]]`, `[[ - && a ]]`, `[[ - || a ]]`, and `[[ a -eq ]] ]]`.
No issue was created because GitHub access is excluded from this task.

The initial draft exposed false accepts around a missing infix operand and a first prefix operand `!` or `(`.
Those were corrected and covered by rejection tests before the final comparison.

## Glob groups in operands

A crash-and-verdict fuzz of the first revision found three introduced false accepts, all an unclosed `(` group in an operand: `[[ -foo a ( b (c) ]]`.
The fork's glob-group lexer ends a group at the first unnested `)`, so the inner `(c)` closed the group and the rest of the line parsed.
Zsh lexes the operand as one word (`gettokstr` in `Src/lex.c` at `zsh-5.9.2`), where a bare `(` nests and `;`, `&`, and a `<` or `>` that starts neither a process substitution nor a numeric glob `<m-n>` end the word, which is then a parse error.
The same lexer rejected valid nesting, `[[ -foo a ( b (c) ) ]]`, and accepted `[[ -foo a ( b ; c ) ]]`.

The fork now reads a group in a `-NAME` condition operand with those rules.
The rules apply only while those operands are read: a nested substitution clears them and each reader restores them, so every other word keeps the existing group lexing.
Pattern operands after `==` have the same native behavior and are unchanged here; `[[ x == a(b;c) ]]` and `print a(b;c)` remain false accepts on both base and fix.

An 80-row group grid (nesting, quoting, substitutions, numeric globs, process substitutions, word breakers) reports 48 fixed, 27 unchanged, 4 still rejected valid sources, and no introduced false accepts.
The still-rejected rows are `${x:-(}` inside a group, bare or in double quotes, and the command-position forms `print a(b(c)d)` and `print *(a|(b|c))`; all four fail the same way on base.
Re-running the 298-row grid (rows 01-48 plus the earlier boundary probes) gives 160 fixed, 132 unchanged, 5 still rejected valid sources, 1 pre-existing false accept, and no introduced false accepts or regressions.
A second seeded fuzz of 2,898 generated conditions through the full CLI had no crash or hang.
Its twelve rows flagged as new false accepts are each a pre-existing false accept (`[[ - ]]`, `[[ x < ! ]]`, `[[ x < ( || ) ]]`) joined by `&&` or `||` to a condition that main rejected and Zsh accepts, so the base verdict came from the other condition.

`-compare -native` over the corpus fixtures and the 1,025 Zsh 5.9.2 shipped function files reports 1,077 unchanged, 42 fixed and 5 moved, with no regressions or introduced false accepts.
The failing set among the shipped functions is the same 60 files as the first revision (101 on base).

Hand mutants of the new lexing, each run against `go test ./syntax/` in the fork:

| Mutant                                                            | Result     |
| ----------------------------------------------------------------- | ---------- |
| Group rules never applied                                         | killed     |
| Rules applied only at nesting depth zero                          | killed     |
| A bare `(` does not nest                                          | killed     |
| `;` and `&` allowed                                               | killed     |
| `<(` and `>(` not read as process substitutions                   | killed     |
| A bare `>` allowed                                                | killed     |
| `<` not read as a numeric glob                                    | killed     |
| Digits, the dash, or the closing `>` of a numeric glob refused    | killed (3) |
| A second dash allowed in a numeric glob                           | killed     |
| A nested substitution keeps the rules, or does not hand them back | killed (2) |
| The infix right operand read without the rules                    | killed     |
| A reader does not restore the rules                               | lived (2)  |

The two surviving mutants change no verdict Zsh disagrees with: leaking the rules to a later `==` pattern rejects only sources Zsh also rejects, as measured over 1,664 generated rows.
The restores keep the change scoped to #484's operands, and the field comment says so.

## Mutation checks

Each new `p.lang.in(LangZsh)` predicate was independently replaced with `true` and `false`.
Every mutant ran `go test ./syntax/` in the fork and `go test ./internal/...` at the root.
All four were killed by test assertions; none lived or timed out.
The original parser bytes were restored and checked after the mutations.

| Gate   | Forced | Fork syntax suite | Internal suite  | Result |
| ------ | ------ | ----------------- | --------------- | ------ |
| infix  | true   | failed (caught)   | passed          | killed |
| infix  | false  | failed (caught)   | failed (caught) | killed |
| prefix | true   | failed (caught)   | passed          | killed |
| prefix | false  | failed (caught)   | failed (caught) | killed |

## Verification

- `bash .github/scripts/agent-setup.sh`: passed.
- `go build ./... && go vet ./... && go test ./...`: passed.
- `(cd third_party/mvdan-sh && go test ./syntax/)`: passed.
- `(cd third_party/mvdan-sh && go test ./syntax/typedjson/)`: passed.
- `GOTOOLCHAIN=go1.26.0 golangci-lint run ./...`: passed, `0 issues.`
- `gofmt -l .`: no changed source files listed; the pre-existing `internal/workflowcontract/regression_corpus_test.go` and its archived base copy remain listed.
- `git diff --check`: passed.
- Corpus runtime: terminates without output and exits zero.
- Parse cost for the new fixture: base and fixed both use one parse with adapter depth zero.

Every Go/lint command used the requested worktree caches and `GOPROXY=off`.
Native subprocesses invoked by existing tests and survey tools were also bounded by a scratch PATH wrapper that executes `timeout 5`.
No sandbox failure blocked a required check.
