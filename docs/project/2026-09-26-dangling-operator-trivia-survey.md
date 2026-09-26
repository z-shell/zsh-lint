# 2026-09-26 dangling operator trivia survey

Recorded per `parser-gap-workflow.md` for the #466 fix: a dangling `&&` or `||` before a closer, with a `\` line continuation or a comment between them.

Front end: the `third_party/mvdan-sh` fork.
Base: `4edace4` (`main`).
Zsh: 5.9.2.

## Why this run exists

#461 made `else` reserved in command position, so the parser now reports a dangling operator before `else` at the `else`.
The dangling-operator adapter then walks back from that closer to the operator, but it stepped over blanks, newlines and comments only.
A `\` line continuation stopped the walk, so F-Sy-H `functions/fsh_theme`, which writes `print ... || \` directly before `else`, parsed on `d2db515` and failed on `2eca4a5`.
A trailing comment after the operator failed the same way on both, because the walk read the comment's text as the operator's operand.

## Reference corpus

The six repositories at the revisions pinned in `corpus-revisions.txt`, extracted with `git archive`, over `corpus-paths.txt`: 18 files.

| Binary              | ok  | failed |
| ------------------- | --- | ------ |
| base (`4edace4`)    | 18  | 0      |
| fixed (this branch) | 18  | 0      |

`-compare -native`: 18 unchanged.
None of the 18 files writes a dangling operator before a closer, so this is a control.

## Workspace survey

`zsh-lint-survey -compare <base> -native` over 1086 Zsh files across the workspace repositories (267 outside vendored trees), the 87 corpus fixtures, and 71 probe rows.

| Result    | Count |
| --------- | ----- |
| FIXED     | 42    |
| MOVED     | 2     |
| REGRESSED | 0     |
| Unchanged | 1200  |

The fixed files are F-Sy-H `functions/fsh_theme` (at `6a022ef`), the new fixture `ok-dangling-and-or-continuation.zsh`, and 40 probe rows.
The two moved files are probe rows, `print a || \` and `print a && \` each followed by a line holding only `;` or `&`.
They now stop at the separator instead of the operator.
The `;` row is native-valid and is the redundant-separator gap #467, which the masked operator reaches.
The `&` row is native-invalid and stays rejected.

Known disagreements left in place: 101 native-valid files failing in both builds and 1 native-invalid file parsing in both.
This fix changes none of them.

## Probe grid

`zsh-lint-probe -bodies` placed 10 bodies (6 dangling-operator shapes that must parse, 4 that must stay rejected) in 26 scanner contexts: 260 files.

| Result             | Count |
| ------------------ | ----- |
| FIXED              | 108   |
| Unchanged          | 152   |
| Known gaps         | 6     |
| False accepts      | 0     |
| Introduced changes | 0     |

The six known gaps are the backquote context, where a dangling operator already fails on `main` with or without trivia; that is #469.
`zsh -f -n` does not parse inside a backquote substitution, so those rows were judged by running them with `zsh -f`.

## Retry cost

`zsh-lint-survey -trace-parses`, base against fixed:

| File                                  | base | fixed |
| ------------------------------------- | ---- | ----- |
| F-Sy-H `functions/fsh_theme`          | 11   | 12    |
| `ok-dangling-and-or-continuation.zsh` | 1    | 10    |
| `ok-dangling-and-or.zsh`              | 9    | 9     |

The adapter masks one operator per pass, as before.
The rises are the sites it now resolves: `fsh_theme` has one more, and the new fixture has nine, where base stopped at the first error after one parse.

## Mutation testing

`.github/scripts/mutation.sh origin/main`: 59 killed, 0 lived, 0 not covered, 2 timed out.
A first run left 12 mutants alive.
Four of them sat on three bounds guards that the callers already made redundant, so the guards were removed.
The rest are killed by direct tests of the walk-back helpers, including the edges of the source.
