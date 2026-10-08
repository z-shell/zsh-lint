---
name: parser-gap-fix
description: Fix a zsh-lint parser gap or false accept end to end, from proving it with both oracles to the pull request's verification table. Use for any change under internal/parse that makes valid Zsh parse or invalid Zsh fail. Advisory; the contract is docs/project/parser-gap-workflow.md.
---

# Parser-gap fix

This skill is the working order for a parser change.
Read the project contract in [`docs/project/parser-gap-workflow.md`](../../../docs/project/parser-gap-workflow.md) and the complete native [parser instructions](../../instructions/parser-front-end.instructions.md) before following the steps below. The project contract owns commands, fixtures, oracle behavior and verification gates; the native instructions deliver the reconciled scoped guidance. Zsh semantics follow the released Zsh manual as the organization's [Zsh scripting instructions](https://github.com/z-shell/.github/blob/9c96960532066b8686b0bf74628f859990a9231b/.github/instructions/zsh/scripting.instructions.md) require.
This skill adds no rule and defers to those contracts. The steps below retain the project execution order and command options.

## 0. Prepare the environment

Run `bash .github/scripts/agent-setup.sh` once per session.
It installs `zsh` when it is missing, installs `golangci-lint` v2.12.2 built with the local toolchain, and warms the module cache.

## 1. Prove the gap with both oracles

Write the smallest script that shows the construct, as a file (not a `-c` string: an unterminated loop header at end of input is valid only in a file).

```sh
go run ./cmd/zsh-lint-survey -judge gap.zsh                      # GAP, FALSE-ACCEPT or AGREE, with both verdicts
go run ./cmd/zsh-lint-survey -reduce big.zsh > gap.zsh           # shrink a real file to the construct
```

- `-judge` prints the `zsh -f -n` diagnostic and the zsh-lint diagnostic and classifies the file: valid Zsh that zsh-lint rejects is a parser gap (`GAP`); invalid Zsh that zsh-lint accepts is a false accept (`FALSE-ACCEPT`).
  It judges `zsh -f -n` by stderr, not exit status (`! true` exits 1 with no diagnostic), and never runs the file.
- `-reduce` shrinks one gap or false accept by lines, then shell tokens and blocks, keeping a candidate only while the class, the first message and the error's place in the original file stay the same; it never runs a candidate.
  `-fixed <binary>` also keeps only candidates that build fixes, to stay on the defect a fix addresses; `-candidate <binary>` judges with another build.
  Check the result by hand: the smallest source with the same message can still be a different construct, and a leftover word shortened to `a` may deserve a meaningful name in the issue.
- `zsh -f -n` skips arithmetic evaluation and assignment-word expansion (#287); a source Zsh rejects only when it runs is runtime-tier (step 3).
- Name the language feature from the released manual (`zshmisc`, `zshexpn`, `zshparam`, `zshoptions`), not from memory, another shell, or what mvdan/sh accepts, and keep one feature per issue.
- Keep the section's URL, `https://zsh.sourceforge.io/Doc/Release/<Page>.html#<Section>`: the issue body and every fixture cite it. When the manual and `zsh -f -n` disagree, the binary decides; record both and `zsh --version` in the issue.

## 2. Check the issue and the backlog

- Search open `parser-gap` issues for the same construct before filing; the survey reports only the first error per file, so a gap is often already open under another file.
- File with the parser-gap issue form; it asks for the minimized script, the `zsh -f -n` output, the zsh-lint diagnostic, and the manual section.

## 3. Add fixtures before the fix

- Every fixture carries a `# Manual: <url>` line; `TestFixturesCiteManual` (`internal/manualcite`) enforces it. A fixture renamed away from a name in `internal/manualcite/testdata/fixture-exemptions.txt` gains the line and leaves the list, and `exemptionCeiling` drops, in the same change.
- Name the language feature from the manual section (`zshmisc`, `zshexpn`, `zshparam`, `zshoptions`) and keep one feature per issue.
- Invalid Zsh that must stay rejected: `internal/parse/testdata/invalid-<issue>-<slug>.txt`, read by a focused parser test that asserts the error family and position.
- A source `zsh -f -n` accepts but Zsh rejects at run time goes in `runtimeTierInvalidFixtures` (`internal/survey/native_oracle_test.go`) with its runtime error. Never execute an invalid source.
- `go test ./internal/survey -run TestCorpusFixturesAgreeWithNativeZsh` re-checks every fixture against `zsh -f -n`.
- `go test ./internal/survey -run TestStructuralOracle` checks that every `ok-*` fixture's tree is the program Zsh reads; a new `ok-*` fixture must pass it, and a fix that makes a fixture listed in `structuralOracleKnownDifferences` agree removes that entry.

## 4. Implement

- Fix the gap in the parser fork (`third_party/mvdan-sh`, ADR-0030), behind `LangZsh`, and list the change in its `FORK.md`; run upstream's tests there (`cd third_party/mvdan-sh && go test ./syntax/`). Never add an adapter. When the construct belongs to an adapter whose family has not moved into the fork, fix that adapter through its shared scanner (`internal/parse/double_quote.go`, `internal/parse/heredoc_scan.go`), or migrate the family.
- In an adapter, follow the invariants: register once in `adapterChain`, retry through `parseWithAdapters`, gate on one construct and one parser error, map every byte back, restore the typed AST, return the parser error when unsure.
- Resolve every site of the construct in one pass; masking one site per pass and re-entering the chain costs one whole-file parse per site (#366).
- For non-trivial scanner or grammar logic, use the organization's [independent verification workflow](https://github.com/z-shell/.github/blob/9c96960532066b8686b0bf74628f859990a9231b/.github/instructions/quality/independent-verification.instructions.md): draft, then verify adversarially against native Zsh.

## 5. Verify

Write the construct's valid and invalid variants to a body file, separated by `---` lines, then run every check in one command:

```sh
bash .github/scripts/verify-parser-change.sh --bodies bodies.txt [--rows rows.txt] \
  [--list files.txt] [--regression-corpus DIR] [base-ref] <file.zsh ...>
```

It runs build, vet and tests, the fork's tests, `golangci-lint` with the toolchain `go.mod` names, a base build exported with `git archive` (default `origin/main`), `-compare -native` over the corpus fixtures and the given files (#412), the probe grid with `-known`, `-trace-parses` on both builds (#408), and `.github/scripts/mutation.sh`.
It prints one line per check, the tail of any failing log, and a table plus the compare, probe, trace and mutation output for the pull request; each check's full log stays in the directory it names.
A file outside the repository appears by its absolute path; shorten it before pasting.
It exits 1 when a check fails, 3 when only the mutation run was inconclusive, and 2 when it cannot start.
`--skip-mutation` leaves out the slowest check while iterating.

- Run it after `git fetch origin` and a rebase: with the default base it refuses to start unless `origin/main` is origin's `main` and the checkout contains it, and it prints both commit ids (#545).
  A named base-ref skips the remote check but not the containment check.
- `--rows FILE` adds a grid of complete sources, one per line, for shapes the probe contexts cannot express, such as words in a subscript.
  `internal/probe/testdata/rows-538.txt` is the grid #539 reported.
- `--root DIR` and `--list FILE` add consumer or workspace files to the comparison; a listed path that is not a file fails the check.
  `--regression-corpus DIR`, a directory holding `F-Sy-H/` and `zsh/` at the revisions `regression-corpus.txt` pins, runs the Corpus Gate's own comparison.
- `--candidate REV` judges another commit's build instead of the working tree, to re-run a merged change's numbers with today's tools; the Go checks and mutation are then skipped.
- `-compare -native` must show no `REGRESSED` and no `FALSE-ACCEPT`; every `FIXED` and `MOVED` line belongs in the pull request.
- A grid is also judged with `-runtime`: a row `zsh -f -n` accepts that the change now rejects, and that Zsh rejects when it runs, is `RUNTIME-REJECTED` rather than `REGRESSED` and does not fail the check.
  These are assignments and command substitution bodies, which `zsh -n` does not check (#519, #539); list the family in the pull request.
  `-runtime` runs the rows, so the script uses it on grids only.
- Each grid's changed rows are written as a survey-record table, `probe.md` (or `probe-N.md`) next to the logs.
- The probe summary counts known gaps and false accepts that the change leaves in place; `-known` lists them, and each one is an issue to file.
- The mutation check runs changed Go source lines, including the parser fork and `case` guards, in isolated snapshots with a timeout and process group per build/test. Fork mutants run the fork's syntax tests and the root parse/survey tests. A survivor fails the check; timeout-dominated, invalid, unsupported or capped runs are inconclusive. Report each result, including surviving and unsupported changes; see the workflow's [mutation contract](../../../docs/project/parser-gap-workflow.md#verification-tools) for extension specs, replay and timeout controls.
- The Parse Cost workflow repeats the retry-cost comparison on the pull request and adds a notice above a 10 percent rise.

## 6. Record and open the pull request

- When a corpus or workspace verdict changes, add a dated survey record under `docs/project/` in the format of the existing ones and index it in `docs/project/README.md`.
- Pull request body: Summary, Change (per file), Verification (the `-compare` lines, the probe counts, the retry-cost numbers, the checks run), and Out of scope (gaps the fix uncovered, each filed as its own issue).
- File every newly exposed gap before merging; the next first error in a file is usually a different feature.
