# 2026-09-24 parser survey (#382)

Recorded per `parser-gap-workflow.md` steps 1 and 5 for the #382 fix, a flagged subscript pattern the parser cut short and then reported, or did not report, according to what the leftover bytes meant in their context.

Front end: `mvdan.cc/sh/v3` v3.14.1.
Base: `07275f2` (`main`, the #379 fix merged as #381).
Zsh: 5.9.2.

## Why this run exists

#382 is the opposite of a parser gap: `zsh-lint` accepted source native Zsh refuses.
That direction produces no diagnostic and no differential, so nothing in the corpus can see it, and the survey's usual job — finding valid Zsh we reject — cannot find it either.
This run therefore exists as a control: it shows that closing the false accepts moved nothing that was already passing.

The evidence for the fix is the probe sets below, not the corpus.

## Method

Two oracles, because one is not sufficient here.

`zsh -f -n` is a **parse** check. In an assignment, a `case` word or a `[[ ]]` test, Zsh parses these rows and reports `bad substitution` only when the expansion is reached, so `zsh -n` alone calls them valid.
Every row is therefore also **run** under `zsh -f`, and a row is invalid when the parse check rejects it or the run reports `bad substitution` or `invalid subscript`.

Dropping the runtime oracle would have hidden the defect in three of the eight contexts probed.

### Probe sets

| Set   | Rows       | Shape                                                      |
| ----- | ---------- | ---------------------------------------------------------- |
| main  | **16,464** | 8 contexts x 4 subscript positions x 6 flags x 86 patterns |
| quote | **1,728**  | 6 contexts x 3 positions x 6 flags x 16 quote placements   |

Contexts: bare command, double-quoted, mid-string, nested in another expansion, assignment, array literal, `case` word, `[[ ]]` test.
Positions: the subscript's own, a second-level subscript, a range endpoint, and inside a nested expansion.
Flags: `i`, `r`, `R`, `k`, `I`, `in:2:`.

`(n:2:)` is deliberately spelled `(in:2:)`. Alone, `n:expr:` leaves the subscript arithmetic, so every row under it fails with `bad math expression` whether its syntax is good or bad, which makes the flag useless as an oracle: `${m[(n:2:)${Z[a]}$(echo x)]}` is a math error while `${m[(in:2:)${Z[a]}$(echo x)]}` prints a value.
An earlier run used the bare spelling and produced 8 phantom regressions that were entirely an artifact of it.

## Results

| Probe | Rows   | False accepts fixed | Regressions | New false accepts | New gaps |
| ----- | ------ | ------------------- | ----------- | ----------------- | -------- |
| main  | 16,464 | **1,854**           | **0**       | **0**             | **0**    |
| quote | 1,728  | **108**             | **0**       | **0**             | **0**    |

All nine rows of the issue's three tables now match native Zsh on both oracles.

Remaining on the main probe: 1,283 false accepts and 197 gaps, all pre-existing and none in this shape.

## Workspace survey

Every `.zsh` file across the workspace repositories, base against fixed.

| Metric             | Count |
| ------------------ | ----- |
| Files surveyed     | 261   |
| Verdict changes    | 0     |
| Error-text changes | 0     |

The seven files that fail do so identically on both binaries, at the same position and with the same text:

| File                                               | Diagnostic (base and fixed)                                        |
| -------------------------------------------------- | ------------------------------------------------------------------ |
| `zi/lib/zsh/install.zsh`                           | `396:62: statements must be separated by &, ; or a newline` (#376) |
| `zi/zi.zsh`                                        | `2910:9: statements must be separated by &, ; or a newline`        |
| `zi/tests/fixtures/public-contract/foreach/zi.zsh` | `25:1: foreach ... end loops are not supported yet` (#214)         |
| `F-Sy-H/F-Sy-H.plugin.zsh`                         | `387:9: invalid func name`                                         |
| `zpmod/tests/command/zpmod_bundle_build.zsh`       | ``27:1: `}` can only be used to close a block``                    |
| `zpmod/tests/command/zpmod_compaudit_cache.zsh`    | ``27:1: `}` can only be used to close a block``                    |
| `zsh-lint/internal/survey/testdata/gap.zsh`        | `` 3:5: unclosed here-document `EOF` `` (an intentional fixture)   |

Because no first error moved, this change unmasks nothing.

## The mechanism

mvdan/sh reads a flagged subscript's pattern as one raw literal and ends it at the first `]`, wherever that byte sits.
Inside a nested expansion that `]` belongs to the inner subscript, so `${m[(i)${Z[a]}]]}` is read with the pattern `${Z[a` and `]]}` left over.
`resolveFlagPatternCuts` (#283) repairs that by masking the nested brackets and reparsing; when the retry fails, the misread tree stands.

What happened next depended on the enclosing context, and that is the defect:

- **Unquoted**, the leftover `]]}` forms a word ending in `}`, which `rejectCloseBraceWords` (#314/#316) refuses. The row is rejected, for the right reason by accident.
- **Inside a double quote**, the same bytes are ordinary string text. No guard looks at them, and the file is accepted although Zsh reports `bad substitution`.

An outer double quote silently selecting the verdict is a rule no user can infer.

## Decidability

The guard never judges the pattern's bytes. It asks only whether the front end managed to read them the way Zsh does, which is decidable from the tree the parse produced.

**Arm 1 — the repair's own failure.** A pattern the bracket scan can bound past the literal's end is one the parser cut, and by the time the guard runs, `resolveFlagPatternCuts` has already tried and failed to repair it.

**Arm 2 — a literal that ends open.** The bracket scan refuses any pattern holding a quote or an unbalanceable bracket, by design: masking a byte hides it from the parser, so a shape whose extent the scanner cannot decide must keep the verdict it has rather than gain one.
But that refusal is decided on the source, while the cut is visible on the tree. A literal ending while still inside a `${` or `$(` it opened is a cut on its face, since a pattern Zsh read whole would close what it opened.

The two overlap without subsuming each other, measured by disabling each in turn and re-running both probes:

| Arm                       | Rows it alone rejects      | Of those, valid |
| ------------------------- | -------------------------- | --------------- |
| bracket scan (arm 1)      | 36 (quote probe)           | 0               |
| ends-open literal (arm 2) | 1,278 (main) + 108 (quote) | 0               |

### The quote asymmetry

Arm 2 excludes a literal holding a **single** quote, and that exclusion is measured rather than defensive symmetry.
Zsh's rule for a quote inside a nested subscript is asymmetric between the quote kinds, verified across contexts and flags and two levels deep:

| Source                       | Native             |
| ---------------------------- | ------------------ |
| `${m[(i)${Y['a b']}]}`       | valid              |
| `${m[(i)${Y["a b"]}]}`       | `bad substitution` |
| `${m[(i)${Z[${Y['a b']}]}]}` | valid              |
| `${m[(i)${Z[${Y["a b"]}]}]}` | `bad substitution` |

So an ends-open literal such as `${Y['a b'` really can be the whole pattern of a valid row, where `${Y["a b"` cannot.
Dropping the exclusion turns 108 valid rows into rejections; keeping it costs nothing, because every row it declines is one arm 1 already reaches or no arm would have fixed.

An earlier revision refused **any** quote, mirroring the scanner. That was the cautious reading, and it left the issue's fourth row unfixed for no measured reason.

The double-quoted spelling that looks similar but is valid — `${m[(i)"${Z[a]}"]}`, quoting the nested expansion rather than a key inside its subscript — cannot be regressed by this arm: mvdan/sh reaches no tree for it at all but fails with `reached EOF without closing quote`, so it is a pre-existing gap the guard never sees.

## A second defect, found while fixing the first

`resolveFlagPatternCuts` retried through `parseWithAdapters`, the bare adapter chain, rather than the path that produced the tree it was repairing.
A file holding an anonymous function invocation parses only through `parseAnonymousFunctionArgs`, so in any such file the retry failed on a construct the original parse had already read, and **the repair silently never ran**.

It was invisible while the misread produced no error. It surfaced the moment the guard began reporting those cuts: `ok-flag-pattern-bracket-before-operator.zsh`, a valid corpus fixture, started failing to parse — the file carries an anonymous invocation on line 36, and all twenty of its cut sites were going unrepaired.

The retry path is now a parameter. `parseFull` passes the full path; the anonymous-invocation island passes the bare chain deliberately, which is what bounds the recursion.

## Mutation testing

Fourteen mutations, each written to compile and to change behavior. Twelve caught:

| Mutation                                                 | Caught by                                                    |
| -------------------------------------------------------- | ------------------------------------------------------------ |
| arm 1 disabled                                           | `RejectsFlagPatternCuts/single-quote-before-nested`          |
| arm 1 accepts `[` as a cut site                          | `RejectsFlagPatternCuts/nested-subscript-stray-close`        |
| arm 2 disabled                                           | `RejectsFlagPatternCuts/substitution-close-bracket`          |
| arm 2 drops the single-quote exclusion                   | `AcceptsUncutFlagPatterns`, `TestLitEndsInsideExpansion`     |
| arm 2 also excludes a double quote                       | `RejectsFlagPatternCuts/double-quoted-key`                   |
| arm 2 ignores `$(`                                       | `RejectsFlagPatternCuts/substitution-close-bracket`          |
| arm 2 ignores `${`                                       | `RejectsFlagPatternCuts/nested-subscript-stray-close`        |
| arm 2 never closes its depth                             | 16 tests, including `TestMinimizedCorpus`                    |
| arm 2 ignores a backslash escape                         | `TestLitEndsInsideExpansion`                                 |
| the guard not wired into `Parse`                         | `RejectsFlagPatternCuts`                                     |
| the repair retry uses the bare chain (the #283 bug back) | `TestMinimizedCorpus`, `RepairRunsBesideAnonymousInvocation` |
| the repair disabled entirely                             | 14 tests                                                     |

Two survive, both classified equivalent by direct measurement rather than by reading:

- **`soleLiteral` accepts a multi-part word.** Probed over all 18,192 distinct rows of both sets plus an adversarial set aimed at splitting the word (`$b`, `${b}c`, `"b"`, backquotes, `$(...)`, `$((...))`, `$~b`, a glob, a space): every flagged pattern came back as exactly one `Lit`, and no `FlagsArithm` carried a non-`Word` X. Kept because the guard reads `lit.Value` as the whole pattern, which is only true at one part.
- **The island retry uses the full path.** The island holds one anonymous invocation's own word list, and an invocation nested inside another is not parseable by either path, so no input makes the island need the fallback. Kept as the structural bound on the recursion.

The first run of this set scored 11 of 14, with `arm 1 disabled` surviving.
The first attempt to kill it added second-level-subscript fixtures — which arm 2 catches too, so the mutant kept surviving and the fixtures proved nothing about arm 1.
Isolating each arm against the oracle found the rows arm 1 alone rejects: patterns holding a single quote, the family arm 2 declines by design.
A survivor is a question about which input distinguishes the code, and only measurement answers it.
