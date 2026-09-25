---
name: parser-gap-fix
description: Fix a zsh-lint parser gap or false accept end to end, from proving it with both oracles to the pull request's verification table. Use for any change under internal/parse that makes valid Zsh parse or invalid Zsh fail. Advisory; the contract is docs/project/parser-gap-workflow.md.
---

# Parser-gap fix

This skill is the working order for a parser change.
It adds no rule: the contract is `docs/project/parser-gap-workflow.md`, the adapter invariants are in `.github/instructions/parser-front-end.instructions.md`, and Zsh semantics follow the released Zsh manual as the organization's `zsh-scripting.instructions.md` requires.
Where this file and those disagree, they win.

## 0. Prepare the environment

Run `bash .github/scripts/agent-setup.sh` once per session.
It installs `zsh` when it is missing, installs `golangci-lint` v2.12.2 built with the local toolchain, and warms the module cache.

## 1. Prove the gap with both oracles

Write the smallest script that shows the construct, as a file (not a `-c` string: an unterminated loop header at end of input is valid only in a file).

```sh
zsh -f -n gap.zsh                       # native verdict: valid when stderr is empty
go run ./cmd/zsh-lint-survey gap.zsh    # zsh-lint verdict
```

- Valid Zsh that zsh-lint rejects is a parser gap; invalid Zsh that zsh-lint accepts is a false accept.
- Judge `zsh -f -n` by stderr, not exit status: `! true` exits 1 with no diagnostic.
- `zsh -f -n` skips arithmetic evaluation and assignment-word expansion (#287); a source Zsh rejects only when it runs is runtime-tier (step 3).
- Name the language feature from the manual section (`zshmisc`, `zshexpn`, `zshparam`, `zshoptions`) and keep one feature per issue.

## 2. Check the issue and the backlog

- Search open `parser-gap` issues for the same construct before filing; the survey reports only the first error per file, so a gap is often already open under another file.
- File with the parser-gap issue form; it asks for the minimized script, the `zsh -f -n` output, the zsh-lint diagnostic, and the manual section.

## 3. Add fixtures before the fix

- Valid Zsh: `internal/survey/testdata/corpus/gap-<issue>-<slug>.zsh`, renamed to `ok-<slug>.zsh` once it parses. Do not add it to `requiredFixtures`.
- Invalid Zsh that must stay rejected: `internal/parse/testdata/invalid-<issue>-<slug>.txt`, read by a focused parser test that asserts the error family and position.
- A source `zsh -f -n` accepts but Zsh rejects at run time goes in `runtimeTierInvalidFixtures` (`internal/survey/native_oracle_test.go`) with its runtime error. Never execute an invalid source.
- `go test ./internal/survey -run TestCorpusFixturesAgreeWithNativeZsh` re-checks every fixture against `zsh -f -n`.

## 4. Implement

- Prefer extending an existing adapter or a shared scanner (`internal/parse/double_quote.go`) over a new adapter. A new adapter grows the composition matrix and the retry cost, and ADR-0023 point 4 schedules a front-end decision once the chain becomes the bottleneck.
- Follow the adapter invariants: register once in `adapterChain`, retry through `parseWithAdapters`, gate on one construct and one parser error, map every byte back, restore the typed AST, return the parser error when unsure.
- Resolve every site of the construct in one pass; masking one site per pass and re-entering the chain costs one whole-file parse per site (#366).
- For non-trivial scanner or grammar logic, use the organization's generator-verifier workflow (`.github/instructions/generator-verifier-workflow.instructions.md` in z-shell/.github): draft, then verify adversarially against native Zsh.

## 5. Verify

```sh
go build ./... && go vet ./... && go test ./...
golangci-lint run ./...    # prefix GOTOOLCHAIN=go<go.mod version> when the setup script says so

# Retry cost on the files the change touches, before and after (#408).
go run ./cmd/zsh-lint-survey -trace-parses <file.zsh>

# Verdict changes against the base, judged by native Zsh (#412).
base=$(mktemp -d)                       # an export, not a second worktree
git archive origin/main | tar -x -C "$base"
(cd "$base" && go build -o survey-base ./cmd/zsh-lint-survey)
go run ./cmd/zsh-lint-survey -compare "$base/survey-base" -native \
  internal/survey/testdata/corpus/*.zsh <workspace Zsh files>
```

- `-compare -native` must show no `REGRESSED` and no `FALSE-ACCEPT`; every `FIXED` and `MOVED` line belongs in the pull request.
- Probe the construct in every context the adapter's scanner could meet (quotes, `$( )`, backquotes, here-documents, arithmetic, function bodies, each loop and conditional form) and compare each row with `zsh -f -n`, in both directions.
- Check the tests are not vacuous: revert each guard the change adds and confirm a test fails.
- The Parse Cost workflow repeats the retry-cost comparison on the pull request and adds a notice above a 10 percent rise.

## 6. Record and open the pull request

- When a corpus or workspace verdict changes, add a dated survey record under `docs/project/` in the format of the existing ones and index it in `docs/project/README.md`.
- Pull request body: Summary, Change (per file), Verification (the `-compare` lines, the probe counts, the retry-cost numbers, the checks run), and Out of scope (gaps the fix uncovered, each filed as its own issue).
- File every newly exposed gap before merging; the next first error in a file is usually a different feature.
