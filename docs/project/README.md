# Project documents

Source-adjacent contracts and records for contributors.
User-facing guides live on the [wiki](https://wiki.zshell.dev/community/zsh_lint).

## Living contracts

These define how work is accepted.
Change them in the same PR as the behavior they describe.

- [Rule policy](rule-policy.md): how a rule qualifies, its ID and severity, the documentation schema, and the shipping checklist.
  The `Rule proposal` issue form mirrors it.
- [Parser-gap workflow](parser-gap-workflow.md): how a parse failure becomes a tracked, minimized fixture; the adapter and composition contract; the front-end strategy (ADR-0023).
  The `Parser gap` issue form mirrors its capture and classify steps.
- [Inline suppression contract](suppression.md): the shared `# zsh-lint disable=<rule-id>` directive.
- [Machine-readable output contract](output-contract.md): the greppable diagnostic line and exit codes.
- [Project configuration](project-configuration.md): `zsh-lint.json` discovery, explicit overrides, and metadata for project-aware rules.
- [Reference corpus](corpus.md): the repositories the survey and corpus gate run over, with `corpus-paths.txt`, `corpus-configs/`, and `configured-corpus-expected.json` as the gate's inputs.

## Dated records

Point-in-time survey runs and decisions.
They are history, not policy; read the newest first and do not update older ones.

- [2026-09-24 flagged pattern in arithmetic survey](2026-09-24-flag-pattern-arithmetic-survey.md)
- [2026-09-24 parser survey](2026-09-24-survey.md)
- [2026-09-23 anonymous-function closer retry cost](2026-09-23-anonymous-closer-retry-cost.md)
- [2026-09-21 parser survey](2026-09-21-survey.md)
- [2026-09-16 organization discovery survey](2026-09-16-survey.md)
- [2026-08-28 organization discovery survey](2026-08-28-discovery-survey.md)
- [2026-08-16 readiness gate](2026-08-16-readiness-gate.md)
- [2026-08-16 corpus-update survey](2026-08-16-corpus-update-survey.md)
- [2026-08-16 survey](2026-08-16-survey.md)
- [2026-08-14 survey](2026-08-14-survey.md)
- [2026-06-12 front-end switch to LangZsh](2026-06-12-langzsh-switch.md) (upstream-first wording superseded by ADR-0023)
- [2026-06-12 front-end comparison](2026-06-12-frontend-comparison.md)
- [2026-06-12 survey](2026-06-12-survey.md)
