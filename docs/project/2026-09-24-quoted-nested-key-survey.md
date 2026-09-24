# 2026-09-24 quoted nested key survey

Recorded per `parser-gap-workflow.md` step 5 for the #384 fix, a single-quoted key inside a nested expansion under a flagged subscript.

Front end: `mvdan.cc/sh/v3` v3.14.1.
Base: `7f5ae7b` (`main`).
Zsh: 5.9.2.

## The gap

`scanFlagPatternBrackets` steps over a nested expansion inside a flagged pattern (#371) and refused any quote it met there, because a byte count and a quote are not composable: a quoted bracket still moves the count, so a quoted `[` can rebalance an unquoted stray `]`.

That refusal is correct for the double-quoted spelling, which native Zsh rejects inside a nested subscript anyway.
It is wrong for the single-quoted one, which native Zsh accepts and runs:

| Source                 | `zsh -f -n` + run  |
| ---------------------- | ------------------ |
| `${m[(i)${Z['a b']}]}` | valid              |
| `${m[(i)${Z["a b"]}]}` | `bad substitution` |

Zsh reads a single-quoted region inside a subscript literally, so its bytes are neither subscript delimiters nor arithmetic operators.
`$(( m[(i)a'(b'] ))` is valid, where the same parenthesis unquoted would end the arithmetic expansion.

## The fix

`maskSingleQuotedKey` steps over a single quote standing **inside the nested subscript's own brackets** and holding no delimiter byte, masking the `,` inside it as the other step-overs do.
A second arm on `unclosedSingleQuote` handles the one cut that is not a bracket: mvdan/sh splits a subscript index at any `,` in the raw literal, so a comma inside the key leaves the fragment after it carrying an unbalanced quote, and the error lands on the key's closing `'` rather than past a premature `]`.

Two positions are deliberately refused, and both cost valid Zsh that stays rejected:

- A quote **outside** the nested subscript's brackets. `${Z[a]'x'}` passes `zsh -f -n` and is `bad substitution` when run, so a parse-only oracle would have accepted it.
- A **delimiter inside** the quotes. Native Zsh's verdict there varies by enclosing construct: `m[(i)${Z['a[b']}]=v` runs clean, `$(( m[(i)${Z['a[b']}] ))` is `bad substitution` at runtime, and `${Z['['}]}` is rejected outright.
  Deciding those needs the position-dependent rule the scanner declines to reproduce, so each keeps the verdict it has on `main`.

## Results at the pinned revisions

| Binary              | ok  | failed |
| ------------------- | --- | ------ |
| base (`7f5ae7b`)    | 18  | 0      |
| fixed (this branch) | 18  | 0      |

Resolved file set: **18 files**, extracted with `git archive` at the revisions in `corpus-revisions.txt`.
Verdict changes: **0**.

The reference corpus cannot confirm this fix and is a control here.
As recorded on 2026-09-21 it yields 0 failures and is exhausted as a gap source; this construct appears in none of the 18 files.

## Workspace survey

Every `.zsh` file across the workspace repositories, base against fixed.

| Metric          | Count |
| --------------- | ----- |
| Files surveyed  | 263   |
| Verdict changes | 0     |

No workspace source uses the construct either.
A search for a flagged subscript holding a quoted nested key returned only `zunit`'s `${line[(i)[\']]}`, which is an escaped quote in a bracket expression — a different shape, already covered by `ok-subscript-flag-bracket-pattern.zsh`.

The evidence for this fix is therefore the generated probe grid below, not corpus incidence.
That is the expected shape of the work now: gaps come from probing the grammar rather than from corpus evidence.

## Tree fixture differential

232 fixtures compared, **1** verdict change: the new `ok-flag-pattern-quoted-nested-key.zsh`, which fails on base and passes on the branch.

## Probe grid

846 rows: 18 enclosing contexts x 47 nested-expansion bodies, each judged against three verdicts.

| Metric                     | Count |
| -------------------------- | ----- |
| Intended fixes             | 257   |
| Introduced false accepts   | 0     |
| Introduced regressions     | 0     |
| Pre-existing false accepts | 51    |
| Remaining gaps             | 69    |

Native verdict is `zsh -f -n` **and** a `zsh -f` run, because a parse check is not a validity check: `${Z[a]'x'}` parses and is `bad substitution` when run.
A shell error is detected from stderr naming the script rather than from the exit code, since `[[ -z x ]]` exits 1 because the test is false — judging that row by exit code alone read as a defect that was not there.

The axes were named before the count was trusted: enclosing context (flag `i`/`r`, range endpoint, range with a second element, assignment, nested outer expansion, second subscript, arithmetic, `for (( ))` header, unflagged subscript, double quotes, `[[ ]]`, bracket expression after, text after, two nested expansions, here-doc, case pattern, array literal) x body (unquoted controls, single-quoted without a delimiter, single-quoted with each delimiter, quote outside the brackets, rebalance attempts, double-quoted twins, unterminated, one level deeper).

The 51 pre-existing false accepts are not this change's: they are rows `main` already accepts.
One family is worth naming because the fix makes it reachable rather than creating it — `${m[(i)pattern,3]}` is `invalid subscript` at runtime for any pattern, quoted or not, and is accepted on base with no quote or nesting present (`${m[(i)a,3]}`).

The 69 remaining gaps are the deliberately refused positions above, plus the arithmetic context, where the enclosing `$(( ))` reaches the pattern through a different arm.

## Mutation testing

7 mutations of the new guards, each required to compile and change behaviour: **7 caught**, 0 survivors.

One survivor in the first run was the masked-comma requirement in `flagPatternQuotedCommaBeforeError`.
Rather than assume a missing test, it was measured: a binary with the guard disabled was diffed against the fixed binary over a 61-row grid, and 4 verdicts differed — `${m[(i)${Z[a]}']}`, a stray quote after a repairable nested expansion, is `bad substitution` natively and became accepted.
The guard was therefore load-bearing and merely untested; that row is now asserted.

## Known limits of this fix

A single-quoted key holding a delimiter byte, and any double-quoted key, remain rejected.
Both are recorded in `TestFlagPatternQuotedNestedKeyKeepsUnchangedVerdicts` so a later fix has a test to flip.

The arithmetic context, `$(( m[(i)${Z['ab']}] ))`, is valid Zsh and remains rejected: that path reaches the pattern through `positionalPatternDecidable`, which refuses any quote for the reasons its own comment records.
Widening it is a separate change with its own decidability argument.
