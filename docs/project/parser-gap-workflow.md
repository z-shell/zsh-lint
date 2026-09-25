# Parser-Gap Workflow

Tracking issue: [#10](https://github.com/z-shell/zsh-lint/issues/10).

How a parser failure found in real Z-Shell code becomes a tracked, minimized regression fixture.

## 1. Capture

Run the survey over the documented corpus (`docs/project/corpus.md`).
Every `FAIL` line comes with a greppable `path:line:col: message` diagnostic.
Record the run as `docs/project/YYYY-MM-DD-survey.md` with a gap-mapping table (see `2026-06-12-survey.md` for the format).
Note that the survey reports only the _first_ parse error per file — later constructs are masked until earlier gaps are fixed.

## 2. Classify

Check the Zsh manual (`man zshexpn`, `man zshparam`, `man zshmisc`) to name the language feature involved.
Read the released manual, not memory, another shell's documentation, or what mvdan/sh accepts.
Prefer the man pages installed with the `zsh` you run as the oracle, since they describe that binary; link the section from the released HTML manual, `https://zsh.sourceforge.io/Doc/Release/<Page>.html#<Section>` (baseline Zsh 5.9.2), in the issue body.
When the manual is silent or disagrees with `zsh -f -n`, the binary decides validity; record the disagreement and the output of `zsh --version` in the issue.
One issue per language feature — split broad "file X fails" findings into narrower feature issues.
Worked examples: [#11](https://github.com/z-shell/zsh-lint/issues/11) parameter-expansion flags and operators, [#13](https://github.com/z-shell/zsh-lint/issues/13) multi-name loops, [#15](https://github.com/z-shell/zsh-lint/issues/15) reverse subscripts, [#16](https://github.com/z-shell/zsh-lint/issues/16) filename-generation patterns, [#53](https://github.com/z-shell/zsh-lint/issues/53) nested parameter expansions.
Label new issues `parser-gap` + `corpus`.

## 3. Minimize

Starting from the failing real file, delete everything unrelated until the smallest script that still reproduces the same parse error remains.
Validate both directions:

    go run ./cmd/zsh-lint-survey <file>   # must FAIL with the same family
    zsh -n <file>                         # must pass — the gap is real Zsh

A fixture that `zsh -n` rejects is a broken script, not a parser gap; never commit one to the native-valid survey corpus.

## 4. Promote to fixture

Add the minimized script as `internal/survey/testdata/corpus/gap-<issue>-<slug>.zsh` with the standard Zsh modeline, a leading comment naming the issue, and a `# Manual: <url>` line linking the manual section from step 2.
`TestFixturesCiteManual` (`internal/manualcite`) requires that line on every `gap-`, `ok-` and `invalid-` fixture and checks that the URL names a page of the released manual.
Fixtures that predate the requirement are listed in `internal/manualcite/testdata/fixture-exemptions.txt`; the list only shrinks, so a fixture that gains a citation leaves it in the same change and `exemptionCeiling` is lowered to the new count.
`TestMinimizedCorpus` (`internal/survey/corpus_test.go`) discovers fixtures by scanning the corpus directory and enforces the naming contract: `gap-<issue>-<slug>.zsh` must fail to parse, `ok-<slug>.zsh` must parse, and any other name is rejected.
There is no fixture count assertion: adding fixtures never requires test edits ([#14](https://github.com/z-shell/zsh-lint/issues/14)).
The `requiredFixtures` list in that test is a frozen baseline against accidental deletion of the corpus; never add a new fixture to it ([#406](https://github.com/z-shell/zsh-lint/issues/406)).

### Native-invalid regression sources

False acceptance defects need the opposite contract: native Zsh rejects the source and zsh-lint must also reject it.
Store a minimized source as `internal/parse/testdata/invalid-<issue>-<slug>.txt` with a `# Manual: <url>` comment line linking the grammar the source violates, and exercise it from a focused parser test.
Use `.txt` deliberately so the repository-wide native Zsh syntax gate continues to require every tracked `.zsh` corpus source to be valid.

Record the native decision with `zsh -f -n`, but never execute an invalid test source.
Parser tests read its bytes and assert the error family and original source position.
A source that `zsh -f -n` accepts and Zsh rejects only when the line runs (arithmetic and assignment-word expansion, [#287](https://github.com/z-shell/zsh-lint/issues/287)) is runtime-tier: list it in `runtimeTierInvalidFixtures` (`internal/survey/native_oracle_test.go`) with the runtime error its issue records instead of executing it.
`TestCorpusFixturesAgreeWithNativeZsh` re-checks every recorded verdict whenever `zsh` is installed, as it is in Go CI: every corpus `.zsh` fixture must pass `zsh -f -n`, and every `invalid-*.txt` source must fail it unless it is listed as runtime-tier.
Do not add exceptions to the native Zsh syntax gate for ordinary `.zsh` files.

## 5. Close the loop

When a front-end change (or a front-end swap, [#17](https://github.com/z-shell/zsh-lint/issues/17)) makes a `gap-*` fixture parse, the test fails loudly.
Rename the fixture to `ok-<slug>.zsh` so it becomes permanent regression coverage, and close the issue with a link to the survey run confirming the originating real file now parses.
If the old name is in `internal/manualcite/testdata/fixture-exemptions.txt`, the renamed fixture gains its `# Manual: <url>` line and the old entry leaves the list in the same change; `TestFixturesCiteManual` fails otherwise.

### Front-end strategy

`zsh-lint` owns its parser coverage.
The mvdan/sh front end is a source of fixes to take and test, not a dependency to wait on ([ADR-0023](https://github.com/z-shell/.github/blob/main/decisions/0023-zsh-lint-parser-front-end-strategy.md)).
This replaces the upstream-first wording in the 2026-06-12 records and supersedes the upstream-first acceptance criterion recorded in #125 (closed; its text is historical).

- Fix a proven valid-Zsh gap locally, without conditioning on an upstream response.
  Prioritize by corpus evidence.
- Link an existing upstream issue as a reference.
  Filing new upstream issues is optional and never appears in acceptance criteria.
- A front-end bump is a parser behavior change.
  Land it with tree-shape assertions for every corpus fixture the release affects and a survey run before and after.
  A `gap-*` fixture that stops erroring must fail on tree shape, not pass vacuously: v3.14.1 turns `foreach ... end` (#214) from a parse error into a silent three-command tree.
- The fork trigger has fired ([ADR-0030](https://github.com/z-shell/.github/blob/main/decisions/0030-zsh-lint-parser-fork-trigger-fired.md)).
  The parser is a fork of mvdan.cc/sh under `third_party/mvdan-sh`, wired with a `replace` directive; `third_party/mvdan-sh/FORK.md` lists every local change.
  Adapters migrate into it one construct family at a time, and each migration removes the adapter it replaces and shows parity on the corpus, the workspace, and a probe grid judged by `zsh -f -n`.
  The brace-form `if`/`elif`/`else` and `while`/`until` bodies were the first family ([#446](https://github.com/z-shell/zsh-lint/issues/446)).
- Do not add an adapter.
  Fix a new gap in the fork, or in the shared scanner of an adapter whose family has not moved yet.
  Fork changes stay Zsh-only, behind `LangZsh`, and upstream's own tests in the fork must keep passing.

### Local compatibility adapters

A narrowly scoped adapter in `internal/parse` may close a proven valid-Zsh gap without changing the selected parser dependency only when all of these hold:

- the released Zsh manual and `zsh -f -n` establish the construct's validity;
- the adapter activates for one language construct and, by default, one exact parser error.
  A construct the parser reads as an ordinary command until its body fails (as `repeat count do ...` did before the parser fork read it, #208, #281) has no exact error to gate on; such an adapter may gate on the error position instead, at or after a site of the construct found by scanning the source, provided the retry is verified: the tree the retry produced must contain the construct's expected node at the site, or the adapter returns the parser error;
- the full-file retry maps every byte back to the original source, either by keeping the original byte length or through a source map;
- every transformed byte is restored in the typed AST before analysis;
- no mask swallows a byte that produces a `*syntax.Comment` node: the parser runs with `KeepComments(true)` and `internal/suppress` reads those nodes for directives, so a shape whose comment would have to be masked keeps the parser error;
- regression tests prove original AST text, diagnostic positions, suppression behavior, and invalid-syntax rejection; and
- the minimized fixture is `ok-*` only after the normal analyzer path passes.

If recognition or restoration is uncertain, return the parser error.
Do not use generic error suppression, recovery ASTs, or a compatibility adapter to absorb a second untracked language feature.

### Adapter composition

Register every adapter in `adapterChain` (`internal/parse/adapter_chain.go`) and route its masked retry through `parseWithAdapters`.
Never call `parseTree` directly for a retry, and never hand-write a list of peer adapters to compose with.

A masked retry is an ordinary parse of a whole file.
Any unrelated Zsh construct elsewhere in that file must still parse, so an adapter that retries through a subset of its peers makes correctness depend on which adapter happens to run first.
That defect shipped once: `parseAfterAlternateIf` named exactly two peers, and 40 of 42 ordered pairs of adapter features failed to parse even though each parsed alone.

Composition is permissive across distinct adapters, but it is not unbounded within one.
Most adapters mask a single occurrence of their feature per pass and rely on re-entering themselves to reach a second occurrence, which is correct.
Re-entry must not widen an adapter's own accepted grammar.
The grouped-case adapter once did: allowed to recurse, it masked one extra `)` per pass and accepted `case x in (x|y))) : ;; esac`, which native Zsh rejects.
An adapter with that risk belongs in the parser fork instead, as the grouped-case pattern now is (#452).

Because the survey reports only the first error per file, this class of defect masks real gaps rather than merely reporting false ones.
Re-run the discovery survey after any adapter change and expect the reported set to shift.

`internal/parse/adapter_chain_test.go` enforces all of this: every ordered pair of adapter features and all features together must parse; original text must be restored; invalid Zsh must still be rejected; self-recursion must not widen the grammar; and every snippet must genuinely require its adapter, so a snippet the base parser already accepts cannot make its cases vacuously pass.
Adding an adapter to the chain extends that matrix automatically.

A bounded local island may use source-mapped synthetic terminators only when the adapter verifies the original AST structure, invalid-syntax behavior, and every source position before returning the complete tree.
This exception does not permit general source rewriting or consuming separators from the full-file retry.

### Verification tools

Every masked retry parses the whole file again, so an adapter that resolves one site per pass costs a parse per site ([#366](https://github.com/z-shell/zsh-lint/issues/366)).
Measure that cost with `go run ./cmd/zsh-lint-survey -trace-parses <file>`, which writes each file's whole-source parse count and deepest adapter retry nesting to standard error ([#408](https://github.com/z-shell/zsh-lint/issues/408)).
State the before and after numbers in a pull request that changes them on a corpus file.
The `Parse Cost` workflow does this on every pull request that touches the parser: it compares the base and the candidate over `z-shell/zi` and the corpus fixtures, writes the table to the job summary, and adds a notice for each file whose parse count rose by more than 10 percent; it never fails ([#414](https://github.com/z-shell/zsh-lint/issues/414)).

Produce a change's verdict table with `zsh-lint-survey -compare <base-binary> -native <files>`, where the base binary is `zsh-lint-survey` built from the pull request's base ([#412](https://github.com/z-shell/zsh-lint/issues/412)).
It prints one line per changed file, `FIXED`, `REGRESSED`, `FALSE-ACCEPT`, `REJECTED`, or `MOVED` (still failing at a different first error), judged by `zsh -f -n`, and exits 1 when anything regressed or a false accept was introduced.
With `-native` the summary also counts the known disagreements the change leaves in place (valid files failing in both builds, invalid files parsing in both); `-known` lists them.
For a probe grid, `zsh-lint-probe -bodies <file> -out <dir>` places each variant of the construct in every scanner context (compound-command bodies, command substitutions with and without double quotes, backquotes, and positions after here-documents and odd quotes) for `-compare` to judge ([#428](https://github.com/z-shell/zsh-lint/issues/428)).
`.github/scripts/mutation.sh [base-ref]` mutates every changed line and exits 1 when a mutant survives ([#425](https://github.com/z-shell/zsh-lint/issues/425)).
The parser-gap fix skill (`.github/skills/parser-gap-fix/SKILL.md`) runs these in order.

### Typed metadata and synthesized nodes

When an upstream AST has no field for a native construct, the parser result may retain source-mapped typed syntax nodes as explicit `parse.File` metadata.
This exception requires the adapter gate and byte-preserving retry above, plus a stable association with the owning AST node.
Consumers must inspect the typed metadata rather than recover masked source text.

A construct with no upstream node at all gets its own node in the parser fork rather than a synthesized upstream shape.
`repeat` was once synthesized as a `WhileClause` whose condition statement was the count, and a rule that walked the tree itself read that count as a command (#281); the fork's `RepeatClause` holds the count as a word.

Each construct's mechanism is documented where it is implemented:

| Construct                                                | Issue        | Tree                                                            | Mechanism                                                        |
| -------------------------------------------------------- | ------------ | --------------------------------------------------------------- | ---------------------------------------------------------------- |
| Anonymous function invocation words                      | release #165 | `File.AnonymousInvocations`                                     | `parse.go` (`AnonymousInvocation`), `anonymous_function_args.go` |
| `${name::=word}`                                         | #216         | `:=` operator plus `File.AssignAlwaysExpansions`                | `assign_always.go`                                               |
| `${name[a][b]}`                                          | #215         | first subscript in `Index`, the rest in `File.SecondSubscripts` | `second_subscript.go`                                            |
| `${name[(r)pattern,expr]}`                               | #277         | standard tree                                                   | `subscript_pattern_after_comma.go`                               |
| `repeat count sublist`                                   | #208, #281   | fork `RepeatClause`                                             | parser fork, `third_party/mvdan-sh/FORK.md`                      |
| `for`, `select`, `if`, `while` short and alternate forms | #211, #459   | standard `ForClause`, `IfClause` and `WhileClause`              | parser fork, `third_party/mvdan-sh/FORK.md`                      |
| `${$(( expr ))}`                                         | #361         | standard `ArithmExp`                                            | `nested_arithmetic.go` (`resolveNestedArithmetic`)               |
| Brace-form `if` and `while` bodies                       | #446         | standard `IfClause` and `WhileClause`                           | parser fork, `third_party/mvdan-sh/FORK.md`                      |
