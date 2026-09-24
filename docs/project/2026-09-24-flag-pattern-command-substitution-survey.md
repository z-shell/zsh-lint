# 2026-09-24 parser survey (#379)

Recorded per `parser-gap-workflow.md` steps 1 and 5 for the #379 fix, a command substitution after a bracket expression in a flagged subscript pattern.

Front end: `mvdan.cc/sh/v3` v3.14.1.
Base: `f99faa6` (`main`).
Zsh: 5.9.2.

## Why this run exists

The workflow requires re-running discovery after an adapter change, because the survey reports only the _first_ parse error per file.
This run confirms the opposite of #373's: nothing shifted.
No file in the reference corpus or the wider workspace uses the construct, so no first error moved and no later gap was unmasked.

The corpus has been exhausted as a gap source since the 2026-09-21 run, and #379 was found by probing the grammar while working on #368, not by corpus evidence.
This record therefore exists to show that nothing already passing moved; the evidence for the fix is the probe sets below and the new fixture.

## Method

Five of the six repositories from `corpus.md` extracted with `git archive` at the revisions pinned in `corpus-revisions.txt`, so the working trees are untouched, then the same checks the `Corpus Gate` workflow performs.
Directory entries expanded with NUL-delimited `find -type f`, so dot-prefixed extensionless function files are included.

| Repository              | Pinned revision |
| ----------------------- | --------------- |
| `src`                   | `d19ae48`       |
| `zd`                    | `35b837d`       |
| `z-a-meta-plugins`      | `ba34790`       |
| `zsh-fancy-completions` | `151c60f`       |
| `zsh-eza`               | `93f6e02`       |

`zunit` is not cloned in this workspace, so `zunit/build.zsh` could not be resolved locally and the file set is **17 files** rather than the workflow's 18.
The `Corpus Gate` workflow checks out all six and covers that file in CI.

## Results at the pinned revisions

| Binary              | ok  | failed |
| ------------------- | --- | ------ |
| base (`f99faa6`)    | 17  | 0      |
| fixed (this branch) | 17  | 0      |

Native failures: 0.

The reference corpus cannot confirm this fix: none of the 17 files puts a command substitution after a bracket expression in a flagged subscript pattern.
It is a control, showing that nothing already passing moved.

## Workspace survey

Every `.zsh` file across the workspace repositories, base against fixed.

| Metric             | Count |
| ------------------ | ----- |
| Files surveyed     | 263   |
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

## Tree fixture differential

216 fixtures compared (`internal/survey/testdata` and `internal/parse/testdata`), 1 verdict change: the new `ok-flag-pattern-command-substitution.zsh`, which fails on base and passes on the branch.

## False accepts

The survey reports only files the parser _rejects_, so it is structurally blind to source that lint wrongly accepts.
This change makes a mask **wider**, which is the direction that can introduce one, so the probe sets were generated rather than hand-picked.

Sub-shapes were enumerated from the grammar first, since a zero from a probe set that lacks a shape reads exactly like an absence of defects (the #373 lesson).
Each row varies flag × pattern shape × substitution body × surrounding context, and each was judged with `zsh -f -n` as the oracle across three binaries so a pre-existing defect cannot be reported as an introduced one:

| Probe set                                                               | Rows   | Introduced false accepts | Introduced regressions | Fixed |
| ----------------------------------------------------------------------- | ------ | ------------------------ | ---------------------- | ----- |
| flag × pattern × substitution × context                                 | 14,000 | 0                        | 0                      | 3,861 |
| the same, with a nested expansion beside the substitution (#371 × #379) | 1,320  | 0                        | 0                      | 317   |
| hand-written rows from the issue plus native edge cases                 | 117    | 0                        | 0                      | 41    |
| flag × pattern × **invalid** substitution body × context (see below)    | 4,320  | 0                        | 0                      | 336   |

Substitution bodies covered: a plain command, a nested parameter expansion, a nested command substitution, a nested arithmetic expansion, a parenthesized list, a body holding a `,`, a quoted body in either quote kind, a body holding each of `[ ] { } ( ) #` quoted and unquoted, an escaped bracket, and an unclosed delimiter in both substitution forms.
Contexts covered: command position, double quotes, an assignment RHS, an assignment target, arithmetic, a length prefix, `[[ ]]`, a suffix operator, an array literal, and a `case` pattern.

### The first three probe sets missed a whole class

The zeros in the first three rows were **not** evidence of a sound repair.
Every substitution body those sets generated was a _valid_ command, so no row could ever exercise what happens when the body is invalid, and the whole class was scored as an absence of defects.
This is the #373 lesson recurring in a place the enumeration did not reach: the shape axis was "which delimiter is in the body", never "is the body a command at all".

Review caught it, and the fourth probe set was added with bodies drawn from the other axis: reserved words (`done`, `fi`, `esac`, `else`), an operator with no left operand (`| echo a`, `&& echo a`), an unterminated compound (`if true`, `echo (`), and an empty group (`echo ()`).
Against the scanner as first written, that set reported **48 introduced false accepts** across 8 distinct shapes, each one native-invalid source that the repair had turned into accepted source.

The cause was structural, not a missing byte in a table.
A substitution's bytes stay inside the flagged pattern's raw literal — that is what lets the pattern parse at all — so nothing downstream ever reads them as the commands they are.
Stepping over a substitution whose parentheses merely balance therefore accepts any bytes at all inside it.

The repair now parses the body itself (`substitutionBodyParses`) and refuses the whole pattern when it is not a command list, and tracks the pattern's own parenthesis depth so a `)` that opens nothing is refused rather than consumed.
A refusal keeps `main`'s error rather than reporting the body's own, because the pattern is the construct under repair and inventing a position inside a substitution the upstream parser never entered would be worse than the error the user already gets.

### Where upstream and Zsh disagree on a lone word

`syntax.LangZsh` parses a bare `else` as an ordinary command; `zsh -f -n` reports `parse error near 'else'`.
Five words diverge this way and are refused by name in `zshIncompleteWords`: `else`, `nocorrect`, `repeat`, `foreach`, `end`.

The inverse was measured too, because a guard that over-refuses rejects valid source just as silently as one that under-refuses.
`in`, `fo`, `time` and `coproc` all exit 0 standalone under `zsh -f -n`, so none of them belongs in that list, and `TestZshIncompleteWords` pins both directions.

### Pre-existing false accepts, not introduced here

The second probe set reports 68 rows that native Zsh rejects and **both** binaries accept, so they are pre-existing and this change does not move them.
All of them put a nested expansion inside a flagged pattern inside an outer double quote, for example `print "${m[(i)${Z[a]}$(echo ])]}"`.

The substitution is not what makes them accept: `print "${m[(i)${Z[a]}]]}"`, with a stray `]` and no substitution at all, is also `bad substitution` natively and also accepted on base.
The unquoted twin of each row is correctly rejected by both binaries.
This is the same family as #374 — a nested subscript whose extent the front end reads differently from Zsh — but #374's rows carry no flagged pattern and need no outer quote, so the quoted-flagged-pattern variant is a distinct shape and is filed separately.

## Decidability

The repair steps over a substitution inside the pattern instead of refusing it, and masks the `,` inside it so mvdan/sh does not split the index at a byte that belongs to the substitution.
Every other delimiter refuses the pattern, because native Zsh's reading of it is not what the containing constructs suggest, measured rather than reasoned:

- A bracket is counted as a subscript delimiter regardless of the substitution and regardless of quoting, so `$(echo [)` and `$(echo "]")` are `bad substitution` while `$(echo "[]")` is valid.
- A parenthesis is counted the same way and is likewise not quote-exempt: `$(echo ")")` is `bad substitution`, so a quoted `)` really does close the substitution.
- A brace, unlike those two, _is_ quote-sensitive: `$(echo })` is a native parse error where `$(echo "}")` is valid.
- A `#` comments out the rest of the line including the delimiter, and the adapter contract forbids masking a byte a `*syntax.Comment` would hold.

An earlier revision of this branch treated a bracket inside the substitution as ordinary text and introduced **9** false accepts, caught by the first probe run before review.
That is why the refusals are byte-level and on sight rather than a balance check at the delimiter.

A second revision — the one that was pushed and opened as a PR — kept those byte-level refusals but still accepted any body whose parentheses balanced, and review found the 48 rows described above.
Two defects in the same scanner, both in the widening direction, both invisible to the probe sets as they stood when each was written: the probes prove only the shapes they enumerate, and a zero from them is a statement about coverage before it is a statement about correctness.

The cost is valid Zsh that stays rejected: a quoted body, a nested expansion beside the substitution, and a `$` or a parenthesis inside the backquoted form.
Those rows keep the verdict they have on `main` and are pinned in `TestFlagPatternCommandSubstitutionKnownLimits` so a later fix has a test to flip.

## Mutation testing

Eleven mutations, each written to compile and to actually change behavior, all caught:

| Mutation                                               | Caught by                                                                     |
| ------------------------------------------------------ | ----------------------------------------------------------------------------- |
| a bracket inside `$( )` treated as ordinary text       | `RejectsNativeInvalid/bracket-in-substitution`, `TestMaskPatternSubstitution` |
| the `,` no longer masked in `$( )`                     | `RestoresLiteral/comma_inside_the_substitution`, `KeepsIndexShape`            |
| quotes allowed inside `$( )`                           | `KnownLimits`, `TestMaskPatternSubstitution`                                  |
| brackets allowed in the backquoted form                | `RejectsNativeInvalid/backquote-bracket`, `TestMaskPatternSubstitution`       |
| the `,` no longer masked in the backquoted form        | `RestoresLiteral/comma_inside_a_grave_accent_substitution`                    |
| the scanner refuses substitutions again (the gap back) | `TestMinimizedCorpus`, `ComposesWithEveryAdapter`                             |
| the body no longer parsed at all                       | `BodyRefusals`, `TestSubstitutionBodyParses`                                  |
| `zshIncompleteWords` no longer consulted               | `BodyRefusals/else_alone`, `TestSubstitutionBodyParses`                       |
| `else` dropped from `zshIncompleteWords`               | `BodyRefusals/else_alone`, `TestZshIncompleteWords`                           |
| the pattern's parenthesis depth not tracked            | `BodyRefusals/stray_close_paren_after_a_closed_group`                         |
| the depth reset after a substitution                   | `StillAccepted/group_left_open_across_the_substitution`                       |

The last two are why the accepted rows carry a bracket expression.
A row without one parses upstream unaided, never reaches the repair, and cannot catch a mutation in it — the first attempt at these tests used such rows and two mutations survived.
