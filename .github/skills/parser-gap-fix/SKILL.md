---
name: parser-gap-fix
description: Fix a zsh-lint parser gap or wrong tree for valid Zsh in organization code or the external corpus, from proving it to the pull request's verification table. Advisory; the contract is docs/project/parser-gap-workflow.md.
---

# Parser-gap fix

Read the [project contract](../../../docs/project/parser-gap-workflow.md) and the complete native [parser instructions](../../instructions/parser-front-end.instructions.md) before following this working order.
The contract owns commands, fixtures, oracle behavior and verification gates; this skill adds no rule.
Zsh semantics follow the released manual through the organization's [Zsh scripting instructions](https://github.com/z-shell/.github/blob/9c96960532066b8686b0bf74628f859990a9231b/.github/instructions/zsh/scripting.instructions.md).

1. Prepare once per session with `bash .github/scripts/agent-setup.sh`; use its printed lint command.
2. Prove and minimize the failure using the contract's [Minimize section](../../../docs/project/parser-gap-workflow.md#3-minimize):

   ```sh
   go run ./cmd/zsh-lint-survey -reduce big.zsh > gap.zsh
   go run ./cmd/zsh-lint-survey -judge gap.zsh
   ```

   Read the reduced source and confirm that `GAP` still identifies the intended language feature, or prove the wrong tree for valid Zsh through the contract's structural oracle.
   Park `FALSE-ACCEPT` issues as the contract requires.
   Read the survey command's help for reduction limits and candidate-build options.

3. Search the parser-gap backlog for that feature and link its owning issue and released-manual section before implementation.
   Use the parser-gap issue form when filing a new issue.
4. Add the failing regression first, following [Promote to fixture](../../../docs/project/parser-gap-workflow.md#4-promote-to-fixture), including its native-invalid, citation and structural-oracle contracts.
5. Implement through the [front-end strategy](../../../docs/project/parser-gap-workflow.md#front-end-strategy) and [adapter invariants](../../instructions/parser-front-end.instructions.md#adapter-invariants).
   Verify non-trivial scanner or grammar logic with the organization's [independent verification workflow](https://github.com/z-shell/.github/blob/9c96960532066b8686b0bf74628f859990a9231b/.github/instructions/quality/independent-verification.instructions.md).
6. Run `.github/scripts/verify-parser-change.sh` as [Verification tools](../../../docs/project/parser-gap-workflow.md#verification-tools) describes and paste its table into the pull request description:

   Put the construct's valid and invalid variants in `bodies.txt`, separated by `---` lines.

   ```sh
   bash .github/scripts/verify-parser-change.sh --bodies bodies.txt [--rows rows.txt] \
     [--list files.txt] [--regression-corpus DIR] [base-ref] <file.zsh ...>
   ```

   Inspect every check's log and the summary; report changed verdicts, probe counts, retry costs and surviving or uncovered mutants.
   Read the [script header](../../scripts/verify-parser-change.sh) for options and prerequisites; `--check-base` verifies the current base and candidate before building.
   `--candidate REV` replays another commit's build and skips Go checks and mutation; `--skip-mutation` is for iteration, not final mutation evidence.
   Its runtime probe executes generated rows only; never use runtime comparison on corpus or consumer sources.
   Report every mutation result, including invalid and unsupported mutants; for changed lines the runner cannot mutate, run `.github/scripts/mutation.sh --spec FILE` per the [mutation contract](../../../docs/project/parser-gap-workflow.md#verification-tools).

7. Put the survey and verification output, changed corpus or consumer verdicts and remaining gaps in the pull request description.
   Add a dated record under `docs/project/` only for a cross-cutting decision, never for an individual parser or rule fix.
   Follow [Close the loop](../../../docs/project/parser-gap-workflow.md#5-close-the-loop); file newly exposed gaps before merging.
