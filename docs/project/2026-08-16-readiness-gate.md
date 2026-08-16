# Organization Rollout Readiness Gate, 2026-08-16

## Decision

The local mixed-worktree candidate satisfies the strict reference-corpus gate:

- 19 files surveyed, 19 parsed, 0 failed;
- 0 error-level diagnostics;
- 0 warning-level diagnostics;
- no suppression directive, ignored path, waiver, or severity downgrade; and
- 4 info and 2 hint diagnostics recorded below, outside the strict gate.

This is implementation evidence, not rollout authority. The changes below are
committed locally, but organization rollout, required-check configuration,
publication, and release work remain out of scope until each owning repository
change is reviewed and published.

## Candidate revisions

| Repository                      | Local branch                    | Base revision | Candidate commit |
| ------------------------------- | ------------------------------- | ------------- | ---------------- |
| `z-shell/zsh-lint`              | `feature-129-rollout-readiness` | `8facdaf`     | pending          |
| `z-shell/src`                   | `feature-180-zsh-lint-warnings` | `9633151`     | `baa5268`        |
| `z-shell/zd`                    | `feature-97-zsh-lint-warning`   | `626f0cb`     | `29a6e85`        |
| `z-shell/z-a-meta-plugins`      | `feature-41-zsh-lint-findings`  | `c88d62a`     | `dabbc80`        |
| `z-shell/zsh-eza`               | `feature-93-zsh-lint-finding`   | `820ff48`     | `93fb8c9`        |
| `z-shell/zsh-fancy-completions` | `feature-51-zsh-lint-findings`  | `5f5a002`     | `734bd1b`        |

The unchanged `z-shell/zunit` corpus file came from the meta-workspace checkout.

## Parser result

The documented command shape from [corpus.md](corpus.md), pointed at the six
candidate worktrees, produced:

```text
19 file(s) surveyed, 19 ok, 0 failed
```

The local front end provides narrowly gated compatibility for these native-Zsh
forms that the pinned parser rejects:

- bare associative subscript keys beginning with a dot or containing the
  punctuation used by the Zi communication keys;
- `${^name}` RC_EXPAND_PARAM syntax;
- pattern-bearing `(i)`, `(I)`, `(r)`, and `(R)` subscripts; and
- composed alternate brace-form `if` and `while` constructs in the annex
  handler.

Each same-width mask is restored in the typed AST. Focused tests cover original
rendered text, later-error positions, malformed forms, multiple keys, and the
full real-file paths.

## Analyzer result

Normal analyzer execution with `--format=json` over the same 19 paths produced:

```json
{
  "files": 19,
  "diagnostics": 6,
  "errors": 0,
  "warnings": 0,
  "infos": 4,
  "hints": 2
}
```

The remaining non-gating diagnostics are transparent:

- 2 `plugin/function-scoped-options` hints in completion functions; and
- 4 `security/eval` info diagnostics in `lib/state.zsh`.

No diagnostic was suppressed or reclassified to obtain this result.

## Source remediation ownership

- Organization parent: [z-shell/.github#504](https://github.com/z-shell/.github/issues/504)
- Parser work: [zsh-lint#15](https://github.com/z-shell/zsh-lint/issues/15),
  [#60](https://github.com/z-shell/zsh-lint/issues/60),
  [#61](https://github.com/z-shell/zsh-lint/issues/61), and
  [#129](https://github.com/z-shell/zsh-lint/issues/129)
- Consumer findings: [src#180](https://github.com/z-shell/src/issues/180),
  [zd#97](https://github.com/z-shell/zd/issues/97),
  [z-a-meta-plugins#41](https://github.com/z-shell/z-a-meta-plugins/issues/41),
  [zsh-eza#93](https://github.com/z-shell/zsh-eza/issues/93), and
  [zsh-fancy-completions#51](https://github.com/z-shell/zsh-fancy-completions/issues/51)

## Publication boundary

Before this evidence can authorize organization rollout, each owning repository
must publish its reviewed change, CI must pass on the published revision, and
the organization rollout must be separately authorized.
