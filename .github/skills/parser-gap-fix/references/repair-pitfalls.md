# Parser repair pitfalls

Use the section matching the changed mechanism after reading the [project contract](../../../../docs/project/parser-gap-workflow.md).
The contract owns validity, fixtures, verification and delivery; these checks help select contrasting cases rather than replace that working order.

## A valid parse with a different printed tree

Run `go test -count=1 ./internal/survey/ -run TestStructuralOracle` and group failures by the printed difference before changing the parser.
Reduce one failing shape, inspect the printer branch and the AST field it reads, then compare that field with its contract in `third_party/mvdan-sh/syntax/nodes.go`.
A successful native syntax verdict does not distinguish a wrong tree from a printer defect.
Keep the [structural oracle's shrinking known-difference list](../../../../docs/project/parser-gap-workflow.md#structural-oracle) as an accepted baseline, not a place to hide a new discrepancy.

Add a shared fork row that fails on the wrong printed form before the repair.
Vary separators, comments, nesting, empty bodies and the corresponding long form; confirm each variant's native placement and verdict before treating it as a control.
Prove the variants discriminate by running the same checks against an exported base tree as well as the candidate, without swapping production files in the working checkout.
The fork printer preserves source layout, whereas Zsh deparsing normalizes it; expected printed text must follow the printer's contract, not an assumed expanded layout.
Retain the focused tree, position and consumer assertions that the shared table does not cover.

## A token or parser reading shared by several contexts

For a new parameter operator, inspect both its lexer tokenization and grammar path.
The same token reader can also see literal operator-like text inside another expansion's argument; include those unchanged readings in the contrast set.
After changing token constants, use the fork's declared generation procedure and inspect the generated diff for only the intended token and index changes.
A generator or toolchain failure is separate evidence, not permission to alter dependency pins inside a parser repair.

For a second typed or scoped reading, define which construct owns that reading and which nested contexts reset it.
Check internally constructed parsers, not only the top-level constructor: a nested reader must inherit the option and propagate its read report, or have a checked reason it cannot reach the construct.
The existing `ZshNestedAssignTargets`, `zshInnerParser` and `zshFoldInner` paths in [parser.go](../../../../third_party/mvdan-sh/syntax/parser.go) illustrate this contract.
Cover enabled and disabled settings at each reachable inner site.
Read a per-parse report before reusing the parser, because reset starts the next parse's state.

When preserving the earlier raw reading is part of the repair's compatibility boundary, retry it only after the typed pass fails and reports that it used the new reading.
An unconditional retry changes work counts and error behavior; preserve the earlier error and spans for a rejected file and prove the fallback with a discriminating regression.
Keep ownership-scoped readers narrow rather than growing a list of surface-shape exceptions after every new nesting failure.

## An adapter gate or masked-byte set changes

Review the complete adapter order and every peer whose inputs can contain the new byte.
The first retry that claims a source can steal another adapter's construct even when each passes alone.
For example, [isBareKeyPunctuation](../../../../internal/parse/subscript_compat.go) excludes negated bracket-expression openers because this retry precedes the pattern-after-comma path.
Run `go test -count=1 ./internal/parse/` immediately after changing that set, then compare base and candidate verdicts across the affected contexts.
Include ordinary arithmetic operators that must retain their meaning, and unbraced keys whose punctuation may instead end a word.
Tie an error-text or positional gate to the construct's own offsets and verify the retried tree, rather than accepting a matching message elsewhere in the file.

For a `[[ ]]` operand reader, include base-accepted open groups cut by a boundary token, not only balanced operands.
Use the existing cases in [zsh_cond_operand_group_test.go](../../../../third_party/mvdan-sh/syntax/zsh_cond_operand_group_test.go) to select controls, including malformed groups that must remain rejected.
Measure changed whole-file retries on repeated sites followed by an unrelated error; a local parse success does not establish bounded work.

## Native verdict and verification inputs

Preserve the consumer's containment and exact file bytes when minimizing; moving a function-body row to top level can change the native verdict through expansion.
Follow the [native oracle rules](../../../../docs/project/parser-gap-workflow.md#3-minimize), including stderr-based syntax judgment and controlled generated runtime probes.
A runtime failure needs an unchanged control that distinguishes the construct from unrelated setup; neither an existing parser rejection nor a failing environment proves native invalidity.

Use an explicit base before consumer files when recording a verification command:

```sh
bash .github/scripts/verify-parser-change.sh --bodies bodies.txt --output ../parser-evidence origin/main consumer.zsh
```

`../parser-evidence` must be a new directory outside the checkout with an existing parent.
The current script also recognizes an omitted base when the next argument ends in `.zsh`; read its argument parser instead of assuming every first positional argument is a revision.
When changing path options, test invocation from a checkout subdirectory as well as its root so caller-relative inputs retain their meaning.
Inspect the final logs and the actual selected mutation packages, especially for fork packages outside `syntax`; report survivors, timeouts, invalid mutants and unsupported coverage separately.
A test filter that matches nothing and a package absent from a campaign supply no evidence for that boundary.

For analyzer false positives rather than parse defects, use [rule policy](../../../../docs/project/rule-policy.md) and [diagnostic-diff verification](../../../../docs/project/diagnostic-diff.md).
Those workflows own precision populations and complete diagnostic identities; parsing success alone cannot establish that a rule correction preserves findings.
