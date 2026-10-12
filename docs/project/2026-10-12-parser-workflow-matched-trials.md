# 2026-10-12 parser workflow matched trials

Tracking: [#565](https://github.com/z-shell/zsh-lint/issues/565).
Plan: [parser workflow pilot plan](parser-gap-workflow-pilot-plan.md), step 4.

Four matched agent trials compared the parser-gap workflow pilot (#652, `78ac5fc`) with the workflow before it (`4a532c3`).
Across them, the pilot did not produce measurably better fixes.
One pair per case cannot separate the workflow from run-to-run variance, so this is evidence against a large effect, not proof of none.

## Results

| Trial | Case                                                  | Outcome                                                                                                                                                                                                                                                            |
| ----- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | #541, brace-form `else` tree (fix `f5ad046` reverted) | Both arms made the upstream fix and passed every check; no difference.                                                                                                                                                                                             |
| 2     | #266, `$((` ambiguity (fix `a0cc95d` reverted)        | Same repair route and the same holdout score. Each arm was right on one input class where the other arm and the merged fix were wrong; those defects became #654. No winner.                                                                                       |
| 3     | #643, try/always after `${#` (open issue)             | Both scored 30 of 30 on a hidden native holdout once a row depending on #503 was excluded. Blind scoring ranked the pilot's fix better: it agreed with native Zsh on 12 more valid inputs. Confounded, see below.                                                  |
| 3b    | #643 again, confounds removed                         | Both scored 30 of 30 again and also fixed the same defect in the declaration brace-close scanner. Blind scoring ranked the baseline's fix slightly better; neither dominated, each covering about 20 inputs the other missed. #655 merged the union of both rules. |

In trial 2 the holdout was the fix's own tests: both arms left 0 root and 3 fork tests failing, all on inputs over 1 KiB that both declared as a read-ahead limit.

| Trial | Pilot arm                                                 | Baseline arm                                                                             |
| ----- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| 1     | 5 min, 30 tool calls                                      | 7 min, 30 tool calls                                                                     |
| 2     | 43 min (about 15 blocked on an approval prompt), 96 calls | 44 min active plus 2 h 36 min waiting on a question, 151 calls (stopped by the turn cap) |
| 3     | 20 min, 58 calls                                          | 12 min, 46 calls                                                                         |
| 3b    | 27 min, 82 calls                                          | 15 min, 65 calls                                                                         |

Process measures:

- Tests first: the pilot arm wrote failing tests before the production edit in every trial.
  The baseline arm did in trials 1 and 3b.
- Verification: both arms ran the probe grid and `verify-parser-change.sh` in trials 3 and 3b.
  In trial 2 only the baseline did, because the pilot's grid command was blocked.
- Unsupported edits and weakened checks: none in any arm.
  Every changed expectation in an existing test matched the upstream change or native Zsh.
- Learning: no recorded lesson applied to these cases, so reuse remains unobserved (plan step 5).
- Time: the pilot arm took longer in trials 3 and 3b.

Confounds in trial 3, removed in 3b:

- The prompt told both arms to "take the more conservative option", meant to replace clarifying questions.
  The baseline cited it when it narrowed its fix.
- Only the pilot arm loaded an installed runtime skill that held a lesson written during trial 1.

Deviations from the plan:

- #572 was replaced by the open #643.
  Its fix deleted about 3,200 lines of adapters that later commits build on, so no faithful pre-fix arm exists.
- Arms started concurrently rather than in alternating order.
- Run transcripts, prompts and scoring logs are retained outside the repository.

## Replay procedure

1. Choose a case whose symptom does not point at its cause and that has more than one plausible repair route.
   A case the structural oracle localizes at once, like trial 1, does not discriminate.
2. Build each arm from its revision as a history-free snapshot.
   `git archive` the base: `4a532c3` for the baseline, the pilot revision for the pilot.
   For a closed issue, revert the fix file by file with a three-way merge against the fix's parent, resolving only mechanical adjacent-line conflicts.
   Remove text that states the answer: the fix's tests, fixtures and `FORK.md` row, shared-table rows, and plan text naming the case.
   Commit the result once as "Import source snapshot", with no remote.
3. Check each arm before the run: both suites pass, `zsh-lint-survey -judge` reports `GAP` for the repro, a leak scan finds no trace of the fix, and the instruction files hash as expected.
4. Prepare a hidden holdout the arms never see.
   For a closed issue, use the fix's own tests and fixtures.
   For an open one, use native rows judged with `-judge`: valid rows the base rejects, valid rows it accepts, and invalid controls.
   Exclude rows that depend on a separate known defect.
5. Run each arm in a fresh session with the same runtime, model, reasoning setting and turn budget, each in its own runtime profile with automatic skill and memory writers off.
   Use neutral directory and profile names.
   The prompt gives the symptom, the native evidence and the rules, not the issue text.
   It names no issue, says that no one answers questions, and asks the agent to decide and explain each choice.
   Approval prompts must not block a run: pre-approve or disable them inside the isolated arm, and scan every command instead.
6. Score after both runs finish.
   Rerun both suites, the repro and the holdout on each arm.
   Give the two diffs and check outputs to an independent reviewer under random labels, who probes related native inputs and ranks them before anyone reads the transcripts.
   Then read the transcripts for the process measures, and report active time separately from waits.
