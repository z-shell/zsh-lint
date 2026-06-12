# Inline Suppression Contract

Tracking issue: [#19](https://github.com/z-shell/zsh-lint/issues/19).

How users silence an intentional finding without weakening unrelated
diagnostics. This is the single shared contract required by the first-wave
rule policy ([#7](https://github.com/z-shell/zsh-lint/issues/7),
`docs/project/rule-policy.md` once
[PR #55](https://github.com/z-shell/zsh-lint/pull/55) merges); rules must
not invent their own escape
hatches. This document defines the contract; the implementation is tracked
separately and must cite this file.

## Syntax

A suppression is a Zsh comment directive:

```text
# zsh-lint disable=<rule-id>[,<rule-id>...] [-- reason]
```

- `<rule-id>` is the stable `category/rule-name` slug (e.g.
  `quoting/unquoted-var`). At least one rule ID is **required** — there is
  no blanket `disable` without IDs, by design.
- Multiple IDs are comma-separated, no spaces.
- Everything after an optional `--` separator is a free-form reason. A
  reason is recommended (and rule docs may require it for `security/*`
  rules) but not enforced by the analyzer.

The directive keyword is `zsh-lint` (the product name), matched
case-sensitively after optional leading whitespace in the comment.

## Scope semantics

Two placements, both line-scoped:

1. **Trailing** — on the same line as the code it suppresses; applies to
   findings whose reported range starts on that line:

   ```zsh
   eval "$cmd"  # zsh-lint disable=security/eval -- input is a static table
   ```

2. **Preceding** — a comment on its own line; applies to findings starting
   on the **next non-comment, non-blank source line**:

   ```zsh
   # zsh-lint disable=quoting/unquoted-var
   print $word_splitting_intended
   ```

There is deliberately **no file-level or block-level scope in the first
wave**: line scope keeps suppressions adjacent to the code they excuse and
prevents drive-by blanket disabling. File/block scope, if ever needed, is a
separate proposal that must amend this contract.

A suppression silences only the listed rule IDs, only within its scope.
Diagnostics from other rules on the same line are unaffected.

## Malformed suppressions

A comment that begins with the `zsh-lint` directive keyword but does not
parse against the syntax above (unknown verb, missing rule list, malformed
rule ID) is **never silently ignored**. The analyzer reports it as a
diagnostic:

- ID: `meta/malformed-suppression`, severity `warning`.
- The malformed directive suppresses nothing.

This guarantees a mistyped suppression surfaces as a finding rather than
as mysteriously reappearing diagnostics.

## Stale suppressions

Staleness is evaluated **per rule ID**, not per directive. Each listed ID
that matches no finding in the directive's scope (the code was fixed, the
rule narrowed, or the ID does not exist in the current rule set) is
reported, even when other IDs in the same directive did suppress findings:

- ID: `meta/unused-suppression`, severity `info`, one finding per unused
  rule ID.
- Unknown rule IDs are called out in the message so renamed/deprecated
  rules are caught.

Stale-suppression reporting keeps escape hatches from accumulating. CI
profiles may raise `meta/*` severities; the analyzer defaults stay as
listed.

## Interaction with machine-readable output (#20)

Suppressed findings are dropped from default human output. The
machine-readable contract (#20) must represent suppression explicitly —
either by omitting suppressed findings or by tagging them
(`"suppressed": true`) — and must include `meta/*` diagnostics, so editor
and CI integrations can audit suppression usage. The choice is deferred to
#20 and must reference this section.

## Out of scope (first wave)

- File-level and block-level scopes.
- Severity overrides via comments (use configuration, not inline
  directives).
- Enabling rules inline (`enable=`) — configuration owns rule activation.
