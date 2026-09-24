# 2026-09-24 flagged pattern in arithmetic survey

Recorded per `parser-gap-workflow.md` step 1 for the #368 fix, a flagged subscript pattern that holds a bracket expression in arithmetic.

Front end: `mvdan.cc/sh/v3` v3.14.1.
Base: `4c690f7` (`main`).
Zsh: 5.9.2.

## Why this run exists

The workflow requires a survey after an adapter change, because the survey reports only the _first_ parse error per file.
This change also widens an existing adapter.
Every earlier arm of the flagged-pattern repair is gated on a known error text.
The new arm is gated on position, because in arithmetic the bytes after the cut `]` are read as arithmetic, and the error text depends on whatever byte the pattern holds there.
So this run also asks whether the positional gate moved any verdict outside the construct it targets.

## Method

The six consumer repositories were extracted with `git archive` at the revisions pinned in `corpus-revisions.txt`, so no working tree was touched.
Directory entries were expanded with NUL-delimited `find -type f`.
Every check ran twice, with a binary built from `4c690f7` and with the fix applied.

Resolved file set: **18 files**, unchanged from the #373 run.

## Results at the pinned revisions

| Binary              | ok  | failed |
| ------------------- | --- | ------ |
| base (`4c690f7`)    | 18  | 0      |
| fixed (this branch) | 18  | 0      |

Native failures: 0.

None of the 18 files uses a flagged pattern with a bracket expression in arithmetic, so this is a control.

## Workspace survey

Every `.zsh` file across the workspace repositories, base against fixed.

| Metric          | Count |
| --------------- | ----- |
| Files surveyed  | 263   |
| Verdict changes | 0     |

The construct has no occurrence in the workspace today.
The issue came from adversarial probing of the #358 flagged-pattern repair, not from a failing file.

## Tree fixture differential

72 fixtures compared, 1 verdict change: the new `ok-flag-pattern-arithmetic.zsh`, which fails on base and passes on the branch.

## Probe grids

The survey cannot see what it does not reject, so two grids were generated and judged with `zsh -f -n` as the oracle.
Each row is a flag from `i r I R e n:2:i b:2:i w`, a pattern from a set of 29, and a context.

| Grid                          | Rows | Contexts                                               |
| ----------------------------- | ---- | ------------------------------------------------------ |
| Arithmetic                    | 3944 | `$(( ))`, `(( ))`, `for (( ))`, assignments, nesting   |
| Parameter expansion (control) | 3944 | `${m[..]}`, `${#m[..]}`, `:` modifiers, quotes, `case` |

| Verdict (`zsh -n`, base, fixed) | Arithmetic | Parameter expansion |
| ------------------------------- | ---------- | ------------------- |
| valid, rejected, now accepted   | 1448       | 224                 |
| valid, rejected, still rejected | 1176       | 336                 |
| valid, accepted, accepted       | 360        | 2184                |
| invalid, accepted, accepted     | 32         | 336                 |
| invalid, rejected, rejected     | 928        | 864                 |
| introduced false accepts        | 0          | 0                   |
| introduced regressions          | 0          | 0                   |

The 224 rows the control grid changed are the same cut outside arithmetic.
Under a length prefix, `${#m[(i)a[bc]]}`, the base parser reports `cannot combine multiple parameter expansion operators`.
Before a `:`, `${m[(i)a[bc]:]}`, it reports `` `:` must be followed by an expression ``.
Both are valid Zsh and both are now read as a flagged pattern; they are covered by unit rows and the fixture.

### Runtime oracle

`zsh -n` does not evaluate arithmetic, so it passes rows that fail when run.
All 334 rows that `zsh -n` passes, the base rejects, and the fixed binary accepts, yet which fail at runtime, were re-run beside a control with the bracket expression replaced by a literal.

| Class                                                        | Rows |
| ------------------------------------------------------------ | ---- |
| Control also fails at runtime: the error is not the pattern  | 322  |
| Control succeeds, row runs fine with a different array value | 6    |
| Control succeeds, row fails                                  | 6    |

The last six are `(e)` and `(w)` flags with a parameter expansion after the bracket expression, `(( m[(e)a[bc]$x] ))`.
`print ${m[(e)a[bc]$x]}` fails at runtime the same way, and main accepts it there, so the runtime error is a property of `(e)` evaluation, not of arithmetic.
It is recorded, not guarded: a static reading cannot tell it from `(( m[(r)a[bc]$x] ))`, which runs.

## Adversarial review

An independent review by the `agy` runtime generated 326 rows and reported 14 false accepts judged by runtime failure.
Re-run against the final binary:

- 4 rows with a backslash inside the pattern, such as `$(( m[(i)a[b\]c]] ))`, are now rejected.
  Natively they are not arithmetic at all: Zsh reads a command substitution holding a subshell and reports `no matches found`.
  This finding is why `positionalPatternDecidable` refuses a backslash.
- 10 rows fail at runtime for reasons their no-bracket twin shares: `(e)[[:digit:]]` is the same `bad output format specification` as `(e)[ab]`, and `m[1,(i)a[bc]]` fails with `operator expected` exactly as `m[1,(i)abc]` does.
  `zsh -n` passes all ten, so the parser's verdict matches the oracle.

## Mutation testing

8 mutations of the new arm and its guards, each required to compile and change behaviour: **8 caught**, 0 survivors.

## Known limits of this fix

These are valid Zsh, `zsh -n` passes them, and they keep the rejection they have on `main`:

- A quote inside the pattern, `$(( m[(i)a[b"x"]] ))`.
  Native Zsh does not count a quoted parenthesis, so a byte count cannot decide where `))` is.
- A command substitution after the bracket expression, `${m[(i)a[bc]$(echo x)]}`.
  This is rejected on `main` in every context, not only arithmetic, and is a separate gap.
- An unbalanced parenthesis inside a bracket expression, `$(( m[(i)a[b)c]] ))`, is a native parse error and stays rejected.

## Lint

`golangci-lint` 2.12.2 under Go 1.26.7 reports three `QF1003` findings in `internal/parse/nested_conditional_pattern.go`, a file this change does not touch.
