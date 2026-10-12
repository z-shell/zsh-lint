---
name: parser-gap-fix
description: Fix a zsh-lint parser gap or wrong tree for valid Zsh in organization code or the external corpus, from proving it to the pull request's verification table. Advisory; the contract is docs/project/parser-gap-workflow.md.
---

# Parser-gap fix

Read the [project contract](../../../docs/project/parser-gap-workflow.md) and the complete native [parser instructions](../../instructions/parser-front-end.instructions.md).
The contract owns the [working order](../../../docs/project/parser-gap-workflow.md#working-order), diagnosis, implementation, fixtures, oracles and evidence; follow it through learning capture and delivery.
Zsh semantics follow the released manual through the organization's [Zsh scripting instructions](https://github.com/z-shell/.github/blob/9c96960532066b8686b0bf74628f859990a9231b/.github/instructions/zsh/scripting.instructions.md).

Prepare once per session with `bash .github/scripts/agent-setup.sh`; inspect its requirements and use its printed lint command.
Build a tight failing loop, trace the owning decision, and choose contrasting cases before adopting a production edit.
Use the contract's shared fork cases and retain focused interface regressions.

```sh
go run ./cmd/zsh-lint-survey -reduce big.zsh > gap.zsh
go run ./cmd/zsh-lint-survey -judge gap.zsh
```

Finish adversarial and native-Zsh review, freeze the diff, then run the consolidated verification command once on that final candidate:

```sh
bash .github/scripts/verify-parser-change.sh --bodies bodies.txt [--rows rows.txt] \
  [--list files.txt] [--regression-corpus DIR] [--output DIR] [base-ref] <file.zsh ...>
```

Inspect every log and retain the summary with changed verdicts, probe counts, retry costs and all mutation outcomes.
The default is a reproducible 30-mutant sample with seed 1; use `--full-mutation` at batch boundaries and explicit supplements through `mutation.sh --spec FILE --spec-only`.
Read the script header for prerequisites: `--check-base` checks revisions without building, `--skip-mutation` supports functional iteration, and `--candidate REV` supplies comparison evidence only because it skips functional checks and mutation.
Its runtime comparison executes generated probe rows only.

Put the causal explanation and verified summary in the PR after the push, following the organization's pull-request runbook.
Check every linked issue example before choosing `Closes`; use `Refs` for partial coverage and name remaining gaps.
Individual fixes need no dated project record; use the contract's [learning and reuse checkpoints](../../../docs/project/parser-gap-workflow.md#learning-and-reuse) in the existing repair evidence.
