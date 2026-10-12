# Parser workflow pilot plan

Status: local pilot implemented; independent review, delivery and agent-effectiveness evaluation pending.
Updated: 2026-10-12.
Tracking: [agent replay evaluation, #565](https://github.com/z-shell/zsh-lint/issues/565).

## Goal and ownership

Reduce unsupported parser edits, repeated investigation and repair rework while preserving native Zsh correctness and downstream behavior.
The [parser-gap workflow](parser-gap-workflow.md) owns the repair procedure; this plan owns rollout, evaluation and decisions about further improvements.
The [organization learning-capture procedure](https://github.com/z-shell/.github/blob/main/runbooks/learning-capture.md) owns lesson promotion.
Keep current progress and continuation in the authorized tracking or handoff surface, with links to retained verification; do not turn this plan into a per-fix journal.

One owner carries each repair through evidence and delivery.
An independent reviewer challenges the candidate; additional implementers or automated orchestration are introduced only when measured benefit justifies them.
No new dependency, learning database, transcript-mining hook or automatic instruction writer is part of this pilot.

## Complete repair workflow

Use the contract's [working order](parser-gap-workflow.md#working-order) for each gap:

1. Establish revisions, native oracle and corpus baseline; retrieve relevant prior lessons before editing.
2. Classify the symptom and affected consumers, distinguish rejection from a wrong tree or position, and identify the owning layer.
3. Minimize without changing the cause, trace the live call path and state transition, and write a failing regression with contrasting cases.
4. Repair the smallest owning rule and challenge affected parsing, printing, position, reuse and consumer interfaces.
5. Finish adversarial and native review, freeze the candidate, and run consolidated verification with bounded mutation and retained evidence.
6. Deliver the causal explanation, linked-example verdicts, limitations and verification under the repository's review policy.
7. Capture reusable corrections in existing evidence, promote executable prevention first, and observe reuse on later applicable work.

## Delivery and evaluation sequence

### 1. Review the local pilot

Review the frozen local diff independently for correctness, useful diagnosis steps, fixture composition, native-confirmation safety, verifier exit semantics and unnecessary instruction duplication.
The existing local adversarial review is supporting evidence; it does not satisfy independent review.
Repair confirmed findings, then refresh affected checks and source identity.
Completion: no unresolved correctness finding in the reviewed candidate, with review provenance and actual checks retained.

### 2. Repair the mutation evidence gap separately

The recorded package-routing finding at base `2e7965c5542961ad80dfd19d0e2990ae444fa98a` identifies `testCommands` in `.github/scripts/mutation.go`: fork changes are routed through syntax and root parser/survey tests, omitting the changed expand package.
Reconfirm against the current source before a separately scoped tooling repair.
Select the affected fork package while preserving necessary root-consumer confirmation; verify routing and replay a representative expand mutant that the owning regression detects.
Classify invalid mutants, behavioral failures, survivors and infrastructure outcomes separately.
Completion: the owning package's tests actually run and detect the demonstrated behavioral change.
Until then, campaigns for affected non-syntax fork files carry this evidence limitation.

### 3. Deliver the reviewed pilot

Prepare an accurate PR description referencing #565 and answering all seven organization instruction-impact questions.
Refresh prepared skill delivery, check links and relevant routing, and distinguish delivery presence from observed runtime invocation.
Signed commit, publication, review-of-record, merge and any parent pin advancement retain their respective authorization and verification steps.
Completion: approved project delivery is verified at its actual revision; local readiness alone does not complete rollout.

### 4. Run matched agent trials

Use three historical cases with different failure modes:

| Case | Diagnostic distinction                                  |
| ---- | ------------------------------------------------------- |
| #266 | Lexical rejection of native-valid input                 |
| #541 | Parsing succeeds with the wrong tree                    |
| #572 | Front-end adapter acceptance versus direct fork support |

The committed #625 replacement candidate supplies an additional integration example; its deterministic replay does not substitute for an agent trial.
For each historical case, start both runs from the same pre-fix source snapshot and give the same symptom, task boundaries and acceptance criteria.
Use separate fresh sessions with the same runtime, model, reasoning setting, tool access, dependency versions and budget; neither session sees the reference patch or the other's findings.
Record exact instruction content identities, source and corpus revisions, native shell and tool versions, and any intervention.
Alternate baseline and pilot run order across cases where practical and disclose residual model variability and historical familiarity.

The primary comparison evaluates the bundled workflow and tooling, so it cannot attribute improvement to prose alone.
If attributing a particular improvement becomes necessary, repeat that case with tooling held constant and vary only the relevant guidance.
Apply the same independent acceptance checks to both outputs: native validity, structural agreement, positions, affected consumers and relevant verification.
Retain unsuccessful runs and budget exhaustion; do not report only accepted outputs or substitute the known-good historical patch.
Completion: three comparable baseline/pilot pairs, reviewer outcomes, limitations and retained run evidence are reported against #565.
The small pilot supports a rollout decision, not a general performance claim.

### 5. Observe learning on subsequent work

Apply the contract's [learning and reuse checkpoints](parser-gap-workflow.md#learning-and-reuse) on the next authorized parser repairs.
Retain corrections only when evidence and future applicability justify them; use the existing task evidence and canonical prevention owner.
For each applicable promoted lesson, observe whether a later task retrieved it before editing, performed the check and repeated the mistake.
Distinguish candidates, locally applied prevention, published guidance and actual reuse.
Completion: at least one later applicable task provides reuse evidence, or the evaluation explicitly remains pending because no applicable task occurred.

### 6. Evaluate and simplify

Review the historical pairs and live reuse observations before adding orchestration, caches or broader instruction text.
Keep useful guidance, consolidate duplicates and revise or remove stale triggers through the owning project's review procedure.
Use the organization's existing learning-capture evaluation cadence for longer-term monitoring; this pilot does not create another schedule.
Completion: recommend retaining, revising or stopping the pilot with correctness, rework and overhead evidence.

## Measurements and acceptance

Retain measurements with existing run evidence rather than creating a new ledger or requiring a report for every ordinary fix.

| Measurement             | Observable evidence                                                                          |
| ----------------------- | -------------------------------------------------------------------------------------------- |
| Correctness             | Independent findings, unresolved defects and consumer verdicts                               |
| Unsupported edits       | Edits made without a causal explanation, identified from retained evidence                   |
| Source churn            | Production patch revisions and discarded edits, separately from regression additions         |
| Rework and intervention | Review repair cycles and substantive maintainer corrections                                  |
| Learning                | Applicable lessons retrieved before editing, checks used and mistakes repeated               |
| Overhead                | Time spent searching, capturing and reviewing lessons                                        |
| Cost                    | Elapsed time and available usage metrics, with unavailable values stated                     |
| Mutation evidence       | Generated, killed, surviving, invalid and timed-out mutants; test routing and campaign scope |

Prefer correctness and fewer repeated mistakes over time or cost savings.
A candidate with unresolved functional defects cannot count as a workflow success.
A zero-mutant campaign provides no mutation coverage; a survivor is investigated and classified rather than automatically equated with a production defect.
Do not infer agent effectiveness from deterministic parser replays, passing tests, instruction-file presence or retrospective claims that a lesson was remembered.

## Current evidence and remaining limitations

The local pilot provides the complete causal repair order, shared fork rows, native Zsh confirmation, bounded mutation defaults and retained verifier identity, commands, logs and summary.
The learning extension adds retrieval before editing, compact correction capture, canonical prevention and observed reuse checkpoints.
Initial frozen-candidate verification passed root build/vet/tests, required-shell fork suites, lint, targeted race checks and scoped document checks.
The 116-file corpus comparison and 156-probe comparison were unchanged, with one pre-existing probe gap.
No eligible production Go mutants were generated for the test/tooling diff, and consumer tracing was skipped because no consumer source changed.
Deterministic pre/post replays for #266, #541, #572 and the committed #625 candidate passed using the existing structural oracle.
These results belong to the initial frozen candidate; retain its identity and supplement the documentation extension with its own checks.
Agent reasoning quality, revision count, maintainer intervention and throughput remain unmeasured.
Actual project hooks, MCP loading and discovery across other runtimes remain unverified.
Legacy Zsh error-case confirmation and organization-source reconciliation are separate follow-ups; the pilot does not claim to settle them.

## Continuation requirements

A continuation names the current source revision and working diff, verified and pending phases, retained evidence, task ownership, the next concrete action and authorization still needed.
Carry one concise progress entry per phase change through the existing handoff procedure.
Before resuming, verify the source, ownership and latest continuation; recorded plans and findings are evidence rather than additional authority.
The next planned phase is independent review of the complete local pilot.
