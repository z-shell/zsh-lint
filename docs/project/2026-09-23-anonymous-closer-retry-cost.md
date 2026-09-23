# 2026-09-23 anonymous-function closer retry cost

Measurement record for #357: parsing `z-shell/zi` `zi.zsh` past line 2910 took 777 s on `main`.
Recorded per `parser-gap-workflow.md`, which asks for a measured growth curve rather than a fix asserted from a profile.

## Method

A scratch copy of `main` counted calls to `parseTree`, the base-parser entry point.
It also recorded the adapter-chain index stack at each call.
A parse count is exact and does not depend on machine load, so the input was minimized against "the count grows with N preceding functions" rather than against wall clock.

An earlier wall-clock delta-debugging run showed why that matters.
Its predicate was a single 4 s timeout, and it accepted a candidate that was only slow under load.
It finished on a 298-line output that parses in about 1 s.

## Reproduction

`N` closed one-line functions, then an anonymous function with an argument inside a brace-form `if` inside a `{` group the file never closes:

```zsh
f1() { print 1; }
# … f2 to fN, the same shape
{
  if [[ x ]] {
    () {
    } a
  }
```

The file is invalid, because native Zsh rejects the unclosed group, so the cost is on the error-return path.
Every row reports the same error at the same relative position.

|    N | base parses, `main` | wall clock, `main` | base parses, fixed | wall clock, fixed |
| ---: | ------------------: | -----------------: | -----------------: | ----------------: |
|    0 |                  77 |               6 ms |                 13 |              3 ms |
|  250 |                 577 |             382 ms |                 13 |             10 ms |
|  500 |               1,077 |           1,397 ms |                 13 |             17 ms |
| 1000 |               2,077 |           5,390 ms |                 13 |             30 ms |
| 2000 |               4,077 |          22,744 ms |                 13 |             80 ms |

On `main` the count grows by 2 per preceding function and each parse is linear in the file, so the total is quadratic in file size.
Fixed, the count is constant and only the single parse of the file grows.

## Cause

`prefixEndsWithAnonymousFunction` validates an anonymous-function candidate on the error path.
It parses the prefix ending at the candidate's `}`, and on each "incomplete construct" error it appends the closer that error names, then retries.
Its bound is `32 + braceOpenerCount(prefix)`, which counts every `{` before the candidate, including braces already closed.

Around a brace-form `if`, the prefix error asks for `fi`, and a brace-form `if` never takes one.
Every retry after the second fails with the same error, and the loop still ran its whole bound, each iteration a full adapter-chain parse of the prefix.
In `zi.zsh` a single candidate ran 463 identical retries.

## Change

The loop stops when a retry fails with exactly the error the previous retry gave.
A closer that changes neither the error text nor its position was not taken, so no further closer can help.
Stopping returns `false`, which the caller reports as the candidate's own error: the same direction as exhausting the bound, a possible false rejection and never a false acceptance.

## Results

| Input                                | `main`                                        | fixed                      |
| ------------------------------------ | --------------------------------------------- | -------------------------- |
| `zi.zsh` at `8448b4a`, 164,116 bytes | 777 s (recorded in #357); more than 60 s here | 3.7 s, `2910:9`, as before |
| 629-line extract of `zi.zsh`         | 5,617 parses, 8.9 s                           | 73 parses, 0.11 s          |

No verdict or output changed on the 183 corpus and parser testdata files.
The unit suite, including every existing retry-bound regression test from #255, passes unchanged.

## Remaining cost, not addressed here

The 3.7 s that remains for `zi.zsh` is 829 base parses.
Only four of them come from this loop.
The rest come from adapters that resolve one site per pass and re-enter the chain: the deepest adapter stack is 57 levels, mostly `parseAssignAlways`, `parseAssociativeSubscript` and `parseSecondSubscript`.
Its growth curve has not been measured, and it is left for a separate issue.
