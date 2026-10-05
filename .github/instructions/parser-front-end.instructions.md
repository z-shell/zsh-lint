---
description: "How parser gaps and compatibility adapters are fixed in zsh-lint's mvdan/sh front end"
applyTo: "internal/parse/**,internal/survey/**,cmd/zsh-lint-survey/**"
---

<!-- PROJECT KNOWLEDGE {"project_revision":"7ff05091a5489becf32069db1738d38ec23c8fd5","project_source_blob":"cb36318050d353acd8624f612fb41544635ebcb4","repository":"z-shell/zsh-lint","revision":"21693189f16af6810e54f04b47789aef76dc5db3","source":"knowledge/domains/tooling/zsh-lint-parser-front-end.md","source_blob":"610500c13d663476012cf7e7ffd13298d7c1897e","target":".github/instructions/parser-front-end.instructions.md"} -->

# Parser Front End

This organization source supplies complete native project guidance through the approved records in `knowledge/project-delivery.json`. Edit the organization source, then publish and approve its revision before regenerating a project consumer; the generated consumer is not independently editable. Project instructions retain their existing authoring ownership until approved source publication, complete delivery and compatibility checks pass. Reconciled against project revision `7ff05091a5489becf32069db1738d38ec23c8fd5`; repository-relative code, command and fixture paths below refer to that project.

`internal/parse` wraps `mvdan.cc/sh/v3/syntax` in its Zsh dialect and closes proven valid-Zsh gaps with local compatibility adapters.
The contract is in [`docs/project/parser-gap-workflow.md`](../../docs/project/parser-gap-workflow.md); this file only routes you there and names the invariants that reviews check.

## Before changing anything

1. Prove the gap with both oracles: `go run ./cmd/zsh-lint-survey -judge <file>` reports `GAP`, with no native `zsh -f -n` diagnostic and a failing analyzer verdict. Judge the native result by stderr rather than exit status, as the project workflow requires (`! true` exits 1 without a diagnostic).
   A file both reject is a broken script, not a gap.
   `zsh -f -n` also runs word expansion on a top-level simple command, but only the lexer inside a function body, so state which placement a row uses ([#287](https://github.com/z-shell/zsh-lint/issues/287), `parser-gap-workflow.md` section 3).
2. Name the language feature from the Zsh manual (`zshmisc`, `zshexpn`, `zshparam`) and link its section in the issue body and in the fixture's `# Manual: <url>` line.
   Read the released manual; do not ground a gap in memory, another shell, or mvdan/sh behavior.
   One issue per feature; label it `parser-gap`.
3. Fix locally by default.
   Upstream `mvdan/sh` is a source of fixes to take and test, not a dependency to wait on ([ADR-0023](https://github.com/z-shell/.github/blob/21693189f16af6810e54f04b47789aef76dc5db3/decisions/0023-zsh-lint-parser-front-end-strategy.md)).
4. Fix in the parser fork, not with a new adapter ([ADR-0030](https://github.com/z-shell/.github/blob/21693189f16af6810e54f04b47789aef76dc5db3/decisions/0030-zsh-lint-parser-fork-trigger-fired.md)).
   The fork is `third_party/mvdan-sh`; keep each change Zsh-only behind `LangZsh`, list it in `third_party/mvdan-sh/FORK.md`, and keep upstream's tests there passing.
   An adapter whose family has not moved into the fork yet may still be fixed in place, through its shared scanner.

## Adapter invariants

- Register the adapter once in `adapterChain` (`adapter_chain.go`) and route its masked retry through `parseWithAdapters`.
  Never call `parseTree` for a retry and never hand-write a list of peer adapters.
- Gate on one language construct and, by default, one exact parser error.
  A construct whose error text depends on its body's first token (as `repeat count do ...` did before the parser fork read it, #208, #281) may gate on the error position at or after a scanned site of the construct instead, and must then verify the retry's tree holds the expected node at that site before returning it.
  The retried source maps every byte back to the original, either by keeping the original byte length or through an explicit source map, and every transformed byte is restored in the typed AST before analysis.
- A construct the upstream tree cannot hold may be carried as `parse.File` metadata beside the closest typed shape (for example `File.AnonymousInvocations`, `File.AssignAlwaysExpansions`, `File.SecondSubscripts`); consumers read the metadata rather than masked source text.
  A construct with no upstream node gets its own node in the parser fork (`RepeatClause`, #281), not a synthesized statement that rules could misread.
- If recognition or restoration is uncertain, return the parser error.
  No generic error suppression or recovery ASTs.

## Fixtures

- Valid Zsh that still fails: `internal/survey/testdata/corpus/gap-<issue>-<slug>.zsh`.
  When it starts parsing, rename it to `ok-<slug>.zsh`; the corpus test discovers fixtures by name.
- Invalid Zsh that must stay rejected: `internal/parse/testdata/invalid-<issue>-<slug>.txt`, asserted from a focused parser test.
  Use `.txt` so the repository-wide `zsh -n` gate never sees it.
- Every new fixture carries a `# Manual: https://zsh.sourceforge.io/Doc/Release/<Page>.html#<Section>` line; `internal/manualcite` enforces it.

## Front-end bumps

A `mvdan/sh` version bump is a parser behavior change, not a routine dependency update.
Land it with tree-shape assertions for every affected fixture and a survey run before and after; a `gap-*` fixture that stops erroring must fail on tree shape, not pass silently.
