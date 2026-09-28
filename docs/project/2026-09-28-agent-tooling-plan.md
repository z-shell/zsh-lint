# 2026-09-28 agent tooling plan

Plan for making parser-gap work cheaper and more reliable for coding agents and contributors (#560).
It combines an inventory of this repository at `a44630a` with a review of external practice.
Only the first step, the verify script, ships with this record; each later step is filed and justified on its own.

## What exists

- The corpus gate (`corpus-gate.yml`) compares verdicts against `configured-corpus-expected.json` at pinned corpus revisions.
  Its expected results are checked in, so a change in pass or fail status shows up as a diff; this is the conformance-snapshot pattern [Oxc's test infrastructure](https://oxc.rs/docs/learn/architecture/test) describes.
- `zsh-lint-survey -compare -native [-known]` classifies verdict changes against a base build by the `zsh -f -n` verdict.
- `zsh-lint-probe` writes a construct into every context an adapter scanner can meet.
- `.github/scripts/mutation.sh` mutates changed lines, with exit 3 for an inconclusive run.
- `.github/skills/parser-gap-fix/SKILL.md` lists the working order, and its verify step was about 17 separate commands.

## Observed gaps

- No single command runs the verify step, so each run repeats it by hand and a step is easy to skip or misread.
- Minimizing a reproducer is manual (step 3 of `parser-gap-workflow.md`), although the survey binary and `zsh -f -n` already form a scriptable predicate.
- `TestCorpusFixturesAgreeWithNativeZsh` checks the corpus fixtures against native Zsh, but no test checks that `invalid-*.txt` sources are rejected by Zsh as well.
- The only fuzz test in the root module, `FuzzValidateConditionalPatterns`, checks error offsets; nothing compares parser verdicts with `zsh -f -n`.
- `mutation.sh` mutates only the root module, so a change in `third_party/mvdan-sh` reports no mutants.
  Changed lines inside a `case` expression also report as not covered, because Go's coverage profile has no block for them.
- `zsh-n.yml` judges `zsh -n` by exit status, while the skill judges `zsh -f -n` by empty stderr, since `! true` exits 1 without a diagnostic.
- `AGENTS.md` lists a layout without `cmd/zsh-lint-probe`, `internal/probe`, `internal/workflowcontract`, `internal/manualcite` and `third_party/mvdan-sh`.

## Plan

Ordered by expected effect on a parser-gap fix.

1. **Verify script** (this change).
   `.github/scripts/verify-parser-change.sh` runs the verify step in one command, keeps each check's log, and prints a table for the pull request.
   [Anthropic's skill authoring guidance](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices) recommends bundling deterministic operations as scripts an agent runs rather than prose it follows, with specific error output that lets it fix a failure without reading the surrounding code.
2. **Native-verdict helper and reproducer reducer.**
   A helper that judges a file the way the skill does: file form, empty stderr, never executing the source.
   A line- or token-level delta-debugging reducer whose predicate keeps a candidate only while Zsh accepts it and the survey reports the same gap.
   Character-level reduction is a poor fit here: [the Fuzzing Book's reducer chapter](https://www.fuzzingbook.org/html/Reducer.html) shows it spending most tests on inputs the program under test rejects outright.
3. **Native-verdict coverage for invalid fixtures.**
   Check every `invalid-*.txt` source against `zsh -f -n`, with a checked-in list of known differences, the way [mvdan/sh `TestParseConfirm`](https://github.com/mvdan/sh/blob/master/syntax/parser_test.go) checks its valid inputs.
   For Zsh, `TestParseConfirm` returns before its `errorCases` table ("we don't confirm errors with zsh yet"), so this goes beyond what the fork inherits.
   An opt-in fuzz target comparing verdicts with `zsh -f -n` could follow; generators that produce inputs already labelled valid or invalid found logic bugs in mksh that coverage-guided fuzzing missed, according to [a 2024 shell-fuzzing study](https://arxiv.org/abs/2408.00433), though that is a single study.
4. **Mutation blind spots.**
   Document or cover the fork module and `case` expressions in `mutation.sh`, so neither reads as clean without evidence.
5. **Instruction surfaces.**
   Keep `parser-gap-fix` thin and pointing at the scripts, and consider a separate testing skill for fixtures, native checks and mutation.
   Correct the `AGENTS.md` layout list.
   Evidence that trimming overview prose from instruction files helps agents is mixed ([a 2026 study of repository context files](https://arxiv.org/abs/2602.11988) found small, non-significant gains at higher cost), so this is maintenance, not a measured gain.
6. **Measurement.**
   Replay at least three already-fixed parser gaps from their pre-fix commits, with and without steps 1 to 5, and compare success, turns and cost before investing further.

Changes to CI behavior, such as judging `zsh-n.yml` by stderr or failing local test runs that lack `zsh`, are separate decisions and are not part of this plan's first step.
