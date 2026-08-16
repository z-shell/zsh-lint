# Parser Survey Run, 2026-08-16 (corpus update)

## Scope

This run re-surveys the full [parser evaluation corpus](corpus.md) after two
changes since the same morning's `2026-08-16-survey.md` record:

1. [#143](https://github.com/z-shell/zsh-lint/pull/143) added `zsh-eza`'s
   `functions/` directory to the corpus. Previously only
   `zsh-eza.plugin.zsh` was covered, so `functions/.zsh-eza` had never been
   run through the parser front end.
2. The three parser gaps that morning report left open as unresolved
   diagnostics — [#61](https://github.com/z-shell/zsh-lint/issues/61)
   (leading-dot associative subscript),
   [#60](https://github.com/z-shell/zsh-lint/issues/60) (rc-expand caret),
   and [#15](https://github.com/z-shell/zsh-lint/issues/15)
   (reverse-subscript pattern) — closed and shipped in the `v1.1.0` release
   (`next` → `main` promotion, #142) between that report and this run.

It is parser-front-end evidence only. It does not claim the corpus is free
of semantic-rule diagnostics.

## Environment

| Component        | Value                                               |
| ---------------- | --------------------------------------------------- |
| Zsh              | `zsh 5.9.2 (x86_64-pc-linux-gnu)`                   |
| Go               | `go1.26.6-X:nodwarf5 linux/amd64`                   |
| `mvdan.cc/sh/v3` | `v3.13.1`                                           |
| zsh-lint base    | `24e5ab5678fba638b1c9f8ceabeb1b7e28cd1e4a` (`next`) |

## Result

```text
20 file(s) surveyed, 20 ok, 0 failed
```

All 20 corpus files parse cleanly, including the newly added
`repos/plugins/zsh-eza/functions/.zsh-eza`.

## Interpretation

This run supersedes the Result table in the same-day `2026-08-16-survey.md`
record: all four diagnostics reported there (the `.za-meta-plugins-before-load-handler`
ternary gap, `zsh-eza.plugin.zsh`'s leading-dot subscript, `.man_glob`'s
rc-expand caret, and `completion.zsh`'s reverse-subscript pattern) no longer
reproduce at this commit — the underlying parser fixes landed and released
as `v1.1.0` after that report was written from an earlier, uncommitted
worktree state. It does not supersede that report's historical revision and
environment details, or the earlier 2026-08-14 record.

The newly added `zsh-eza/functions` corpus entry parses cleanly on its first
survey; no new tracking issue is needed from this run.
