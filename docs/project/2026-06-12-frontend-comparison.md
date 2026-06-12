# Front-End Comparison: mvdan/sh vs tree-sitter-zsh — 2026-06-12

Tracking issue: [#17](https://github.com/z-shell/zsh-lint/issues/17).

Records the measured comparison between the `mvdan/sh` front end and the
active [`georgeharker/tree-sitter-zsh`](https://github.com/georgeharker/tree-sitter-zsh)
grammar on the documented survey corpus (`docs/project/corpus.md`),
consolidating the 2026-05-17 run and the 2026-06-12 LangZsh results so a
future front-end ADR starts from evidence rather than assumptions.

## Runs compared

| Run        | Front end                           | Corpus  | Parsed | Failed |
| ---------- | ----------------------------------- | ------- | -----: | -----: |
| 2026-05-17 | mvdan/sh v3.13.1 `LangBash`         | 16-file |      7 |      9 |
| 2026-05-17 | [tree-sitter-zsh](https://github.com/georgeharker/tree-sitter-zsh) [`86b37f8`](https://github.com/georgeharker/tree-sitter-zsh/commit/86b37f8) | 16-file | 6 | 10 |
| 2026-06-12 | mvdan/sh v3.13.1 `LangBash`         | 19-file |      6 |     13 |
| 2026-06-12 | mvdan/sh v3.13.1 `LangZsh`          | 19-file |     11 |      8 |
| 2026-06-12 | mvdan/sh master `LangZsh` (preview) | 19-file |     13 |      6 |

The 2026-05-17 fixture-by-fixture tree-sitter table was recorded in a
working document that was never committed; the aggregate counts and the
notable per-file differences below are what survives from the run log in
[#17](https://github.com/z-shell/zsh-lint/issues/17). A future re-run must
regenerate per-file data against the then-current grammar revision.

## Notable per-file differences (2026-05-17 run)

- tree-sitter parsed `src/public/zsh/init.zsh` (documented `always` block)
  where `LangBash` failed; under `LangZsh` this file still fails, but on
  the brace-form `if [[ ... ]] {` family
  ([#12](https://github.com/z-shell/zsh-lint/issues/12)) rather than on
  `always`.
- tree-sitter failed on `zd/docker/utils.zsh` and `zunit/build.zsh`, both
  of which `mvdan/sh` parses (under `LangBash` and `LangZsh` alike).

## Decision

`mvdan/sh` `LangZsh` stays the front end:

- **Parser fidelity.** On the same 2026-05-17 16-file run, `mvdan/sh`
  led head-to-head even as `LangBash` (7/16 vs tree-sitter's 6/16). On
  the current 19-file corpus, `LangZsh` reaches 11/19 (2026-06-12 run; a
  larger corpus than the tree-sitter figure, so the two numbers are not
  directly comparable). The upstream master preview reaches 13/19, and
  the remaining gaps map to open upstream issues
  ([mvdan/sh#1211](https://github.com/mvdan/sh/issues/1211),
  [mvdan/sh#1297](https://github.com/mvdan/sh/issues/1297)).
- **AST usefulness.** `mvdan/sh` yields a typed Go AST that the analyzer
  and all five shipped rules already consume; tree-sitter would add a
  CGo/FFI boundary and an untyped tree for no current fidelity win.
- **Maintenance.** The upstream-first strategy (track and contribute
  mvdan/sh issues, bump on release) has already retired gaps #11, #16,
  #53 and the function-body case of #12 without local preprocessing
  (`docs/project/2026-06-12-langzsh-switch.md`).

tree-sitter-zsh remains **tracking-only**. Re-evaluate — a fresh per-file
run, then an ADR — if mvdan/sh ships two consecutive releases without
movement on the tracked gaps, or if a rule requires error-recovering
parses of files the front end rejects outright.
