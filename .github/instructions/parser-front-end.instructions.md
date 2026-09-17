---
description: "How parser gaps and compatibility adapters are fixed in zsh-lint's mvdan/sh front end"
applyTo: "internal/parse/**, internal/survey/**, cmd/zsh-lint-survey/**"
---

# Parser Front End

`internal/parse` wraps `mvdan.cc/sh/v3/syntax` in its Zsh dialect and closes proven valid-Zsh gaps with local compatibility adapters. The contract is in [`docs/project/parser-gap-workflow.md`](../../docs/project/parser-gap-workflow.md); this file only routes you there and names the invariants that reviews check.

## Before changing anything

1. Prove the gap with both oracles: `zsh -f -n <file>` passes and `go run ./cmd/zsh-lint-survey <file>` fails. A file both reject is a broken script, not a gap.
2. Name the language feature from the Zsh manual (`zshmisc`, `zshexpn`, `zshparam`). One issue per feature; label it `parser-gap`.
3. Fix locally by default. Upstream `mvdan/sh` is a source of fixes to take and test, not a dependency to wait on ([ADR-0023](https://github.com/z-shell/.github/blob/main/decisions/0023-zsh-lint-parser-front-end-strategy.md)).

## Adapter invariants

- Register the adapter once in `adapterChain` (`adapter_chain.go`) and route its masked retry through `parseWithAdapters`. Never call `parseTree` for a retry and never hand-write a list of peer adapters.
- Gate on one language construct and, by default, one exact parser error. A construct whose error text depends on its body's first token (`repeat count do ...`, #208) may gate on the error position at or after a scanned site of the construct instead, and must then verify the retry's tree holds the expected node at that site before returning it. The retried source maps every byte back to the original, either by keeping the original byte length or through an explicit source map, and every transformed byte is restored in the typed AST before analysis.
- A construct the upstream tree cannot hold may be carried as `parse.File` metadata beside the closest typed shape (for example `File.AnonymousInvocations`, `File.AssignAlwaysExpansions`, `File.SecondSubscripts`, `File.RepeatLoops`); consumers read the metadata rather than masked source text. A statement the front end synthesizes (the `repeat` count condition) is not fed to rules by the analyzer's shared walk; its expansions still are, and a rule that runs its own walk is not covered.
- If recognition or restoration is uncertain, return the parser error. No generic error suppression or recovery ASTs.

## Fixtures

- Valid Zsh that still fails: `internal/survey/testdata/corpus/gap-<issue>-<slug>.zsh`. When it starts parsing, rename it to `ok-<slug>.zsh`; the corpus test discovers fixtures by name.
- Invalid Zsh that must stay rejected: `internal/parse/testdata/invalid-<issue>-<slug>.txt`, asserted from a focused parser test. Use `.txt` so the repository-wide `zsh -n` gate never sees it.

## Front-end bumps

A `mvdan/sh` version bump is a parser behavior change, not a routine dependency update. Land it with tree-shape assertions for every affected fixture and a survey run before and after; a `gap-*` fixture that stops erroring must fail on tree shape, not pass silently.
