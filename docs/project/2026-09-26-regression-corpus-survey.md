# 2026-09-26 regression corpus survey

Recorded for #470: the baseline of the two sources the new `Regression corpus` job in `corpus-gate.yml` compares on every parser pull request.

Tooling: `zsh-lint-survey` built from `main` at `c8dad37`.
Zsh: 5.9.2.

## Why this run exists

#461 (`2eca4a5`) made F-Sy-H `functions/fsh_theme` stop parsing (#466), and neither the reference corpus nor Parse Cost saw it, because F-Sy-H was outside both.
A 2026-09-26 workspace sweep found 102 native-valid failures, almost all in F-Sy-H and in the upstream Zsh `Completion/` tree that `z-shell/zpmod` vendors.
Neither source can join the strict corpus: both still hold open parser gaps.

## Sources

| Path     | Repository       | Revision                                       | Roots                                             | Files |
| -------- | ---------------- | ---------------------------------------------- | ------------------------------------------------- | ----- |
| `F-Sy-H` | `z-shell/F-Sy-H` | `d1856c3` (`main`)                             | `F-Sy-H.plugin.zsh`, `chroma`, `functions`, `lib` | 54    |
| `zsh`    | `zsh-users/zsh`  | `19767e1` (zpmod's `vendor/zsh` submodule pin) | `Completion`                                      | 1029  |

## Strict gate checks on F-Sy-H

The four checks `corpus-gate.yml` applies to the reference corpus, over the 54 F-Sy-H files:

| Check                            | Result                                                     |
| -------------------------------- | ---------------------------------------------------------- |
| `zsh -f -n`                      | 54 valid                                                   |
| `zsh-lint-survey`                | 51 ok, 3 failed                                            |
| `zsh-lint --no-config`, errors   | 3 (the parse failures)                                     |
| `zsh-lint --no-config`, warnings | 148 (147 `quoting/unquoted-var`, 1 `plugin/fpath-hygiene`) |

F-Sy-H therefore joins the regression corpus, not the strict one.
Its three failures are new gaps, each filed:

| File                              | Construct                                                              | Issue |
| --------------------------------- | ---------------------------------------------------------------------- | ----- |
| `chroma/_fsh_chroma_ionice:77`    | `--(class(data\|)\|(u\|p(g\|))id))`, a nested group after leading text | #481  |
| `chroma/_fsh_chroma_nice:87`      | `(#b)(--adjustment)(...))`, a pattern that opens with a globbing flag  | #482  |
| `functions/_fsh_make_targets:138` | `(include[ $TAB]*)`, a blank inside a bracket expression               | #483  |

## Baseline

`zsh-lint-survey -compare <main> -native -known` over both sources, `main` against itself:

```text
1083 file(s) compared, 1083 unchanged; known: 114 gap(s), 1 false accept(s)
```

| Source     | Native-valid | Known gaps | Known false accepts |
| ---------- | ------------ | ---------- | ------------------- |
| F-Sy-H     | 54           | 3          | 0                   |
| Completion | 1026         | 111        | 1                   |

The false accept is `Completion/Base/Utility/_pick_variant`, which `zsh -f -n` rejects only because it expands `${(P)opts[-r]::=$1}` at top level; that is the native-gate artifact #287, not a parser defect.

The largest Completion families by first error:

| First error                                                                                                   | Files | Issue              |
| ------------------------------------------------------------------------------------------------------------- | ----- | ------------------ |
| A module condition with an operand, such as `[[ -prefix - ]]` (47 of the 48 `not a valid test operator` rows) | 47    | #484               |
| `case $x; in`                                                                                                 | 9     | #485               |
| `case patterns must consist of words` and `` `)` can only be used to close a subshell ``                      | 10    | 8 are #481 or #482 |

The rest are single-digit families; the survey reports only the first error per file, so more are masked behind these.

## The gate would have caught #461

`zsh-lint-survey` built from `2eca4a5` compared with its parent, over both sources:

```text
REGRESSED corpus/F-Sy-H/functions/fsh_theme
1083 file(s) compared, 1077 unchanged, 3 fixed, 1 regressed, 2 moved; known: 120 gap(s), 1 false accept(s)
```

`.github/scripts/regression-corpus.sh` exits 1 on that report, and 0 on the `main` baseline above.

## Decision on the vendored Completion tree

It joins as a regression source, not as a strict corpus member and not as a non-gating survey.
A regression source costs nothing for the gaps it already has, because a file failing on both builds passes, and it turns every one of its 915 parsing files into a guard against a change that breaks them.
It is checked out from `zsh-users/zsh` with a sparse checkout at the commit zpmod pins, rather than through zpmod's submodule, so the job does not depend on zpmod's branch layout.
