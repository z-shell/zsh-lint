# Front-End Switch to LangZsh — 2026-06-12

Tracking issues:
[#11](https://github.com/z-shell/zsh-lint/issues/11),
[#53](https://github.com/z-shell/zsh-lint/issues/53)
(strategy decision they both required), with direct impact on
[#12](https://github.com/z-shell/zsh-lint/issues/12),
[#15](https://github.com/z-shell/zsh-lint/issues/15),
[#16](https://github.com/z-shell/zsh-lint/issues/16),
[#17](https://github.com/z-shell/zsh-lint/issues/17).

## Decision

`internal/parse` now parses with `syntax.LangZsh` instead of
`syntax.LangBash`. mvdan/sh gained a dedicated Zsh dialect in the v3.13
line (upstream umbrella
[mvdan/sh#120](https://github.com/mvdan/sh/issues/120), closed), which the
earlier survey work predated.

## Measured evidence (19-file corpus, `docs/project/corpus.md`)

| Front end                                    | Parsed | Failed |
| -------------------------------------------- | -----: | -----: |
| mvdan/sh v3.13.1 `LangBash` (previous)       |      6 |     13 |
| mvdan/sh v3.13.1 `LangZsh` (this PR)         |     11 |      8 |
| mvdan/sh master `LangZsh` (preview)          |     13 |      6 |
| tree-sitter-zsh `86b37f8` (#17, 16-file run) |      6 |     10 |

The master preview (`v3.13.2-0.20260611081601`) additionally fixes
reverse subscripts (#15) and one statement-separation case; those arrive by
staying on the upstream release train rather than via local workarounds.

## Effect on tracked gaps

| Issue | Construct                              | Status under LangZsh v3.13.1                                                                                                                                                                                    |
| ----- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| #11   | expansion flags `${(kv)x}`, `${+name}` | **fixed** — fixture promoted to `ok-param-expansion-flags.zsh`                                                                                                                                                  |
| #53   | ZERO idiom nested expansions           | **fixed** — `ok-nested-param-expansion.zsh`; all three plugin entry files now parse past their ZERO line                                                                                                        |
| #12   | `}` without preceding terminator       | **fixed for function bodies** (`ok-brace-termination.zsh`); the remaining family is zsh brace-form `if [[ ... ]] {` and `} always {` blocks (upstream [mvdan/sh#1211](https://github.com/mvdan/sh/issues/1211)) |
| #16   | glob patterns `(^zunit)`               | **fixed** — `ok-glob-patterns.zsh`; `zunit/build.zsh` parses fully                                                                                                                                              |
| #13   | multi-name `for`                       | still failing; upstream [mvdan/sh#1297](https://github.com/mvdan/sh/issues/1297) is open                                                                                                                        |
| #15   | reverse subscripts `[(I)pat]`          | still failing on v3.13.1; fixed on upstream master                                                                                                                                                              |

## Survey after the switch (LangZsh, v3.13.1)

```text
FAIL z-a-meta-plugins/functions/.za-meta-plugins-before-load-handler
z-a-meta-plugins/functions/.za-meta-plugins-before-load-handler:12:23: statements must be separated by &, ; or a newline
OK   z-a-meta-plugins/functions/.za-meta-plugins-meta-cmd
OK   z-a-meta-plugins/functions/.za-meta-plugins-meta-cmd-help-handler
FAIL z-a-meta-plugins/z-a-meta-plugins.plugin.zsh
z-a-meta-plugins/z-a-meta-plugins.plugin.zsh:11:25: statements must be separated by &, ; or a newline
FAIL src/public/zsh/init.zsh
src/public/zsh/init.zsh:133:5: statements must be separated by &, ; or a newline
OK   zd/docker/utils.zsh
OK   zd/docker/zshenv
OK   zd/docker/zshrc
FAIL zsh-eza/zsh-eza.plugin.zsh
zsh-eza/zsh-eza.plugin.zsh:24:16: `[` must be followed by an expression
OK   zsh-fancy-completions/functions/.complete_menu
OK   zsh-fancy-completions/functions/.completion-prediction
OK   zsh-fancy-completions/functions/.expand-or-complete-with-dots
OK   zsh-fancy-completions/functions/.force_rehash
FAIL zsh-fancy-completions/functions/.man_glob
zsh-fancy-completions/functions/.man_glob:21:13: this expansion operator is a bash feature; tried parsing as zsh
OK   zsh-fancy-completions/lib/compatibility.zsh
FAIL zsh-fancy-completions/lib/completion.zsh
zsh-fancy-completions/lib/completion.zsh:110:87: `*` must follow an expression
FAIL zsh-fancy-completions/lib/state.zsh
zsh-fancy-completions/lib/state.zsh:84:3: `for foo` must be followed by `in`, `do`, `;`, or a newline
FAIL zsh-fancy-completions/zsh-fancy-completions.plugin.zsh
zsh-fancy-completions/zsh-fancy-completions.plugin.zsh:27:5: statements must be separated by &, ; or a newline
OK   zunit/build.zsh

19 file(s) surveyed, 11 ok, 8 failed
```

## Remaining gap mapping

| File                                                                                                                   | Failing construct                          | Tracking                                                                                                                            |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| `z-a-meta-plugins/...` (entry:11, handler:12), `src/public/zsh/init.zsh:133`, `zsh-fancy-completions/...plugin.zsh:27` | brace-form `if [[ ... ]] {` / `} always {` | [#12](https://github.com/z-shell/zsh-lint/issues/12) (re-scoped); upstream [mvdan/sh#1211](https://github.com/mvdan/sh/issues/1211) |
| `zsh-fancy-completions/lib/state.zsh:84`                                                                               | multi-name `for`                           | [#13](https://github.com/z-shell/zsh-lint/issues/13); upstream [mvdan/sh#1297](https://github.com/mvdan/sh/issues/1297)             |
| `zsh-fancy-completions/lib/completion.zsh:110`                                                                         | reverse subscript `[(I)...]`               | [#15](https://github.com/z-shell/zsh-lint/issues/15); fixed on upstream master                                                      |
| `zsh-eza/zsh-eza.plugin.zsh:24`                                                                                        | `${+functions[.zsh-eza]}` inside `(( ))`   | new minimization candidate                                                                                                          |
| `zsh-fancy-completions/functions/.man_glob:21`                                                                         | `${^manpath}` (rc-expand caret)            | new minimization candidate                                                                                                          |

## Consequences

- All five semantic rules pass their existing tests unchanged under
  LangZsh.
- The survey fixture `gap.zsh` (previously `${+commands[git]}`, which now
  parses) was replaced with a multi-name `for` reproduction so the survey
  tests keep exercising a real failure.
- Upstream-first strategy: remaining gaps map to open mvdan/sh issues;
  prefer tracking/contributing upstream over local preprocessing. Bump to
  the next mvdan/sh release when it ships to pick up #15 and further
  statement-separation fixes.
- tree-sitter-zsh (#17) is now measurably behind on this corpus and stays
  a tracking-only option.
