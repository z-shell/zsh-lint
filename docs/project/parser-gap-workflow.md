# Parser-Gap Workflow

Tracking issue: [#10](https://github.com/z-shell/zsh-lint/issues/10).

How a parser failure found in real Z-Shell code becomes a tracked, minimized
regression fixture.

## 1. Capture

Run the survey over the documented corpus (`docs/project/corpus.md`). Every
`FAIL` line comes with a greppable `path:line:col: message` diagnostic.
Record the run as `docs/project/YYYY-MM-DD-survey.md` with a gap-mapping
table (see `2026-06-12-survey.md` for the format). Note that the survey
reports only the _first_ parse error per file — later constructs are masked
until earlier gaps are fixed.

## 2. Classify

Check the Zsh manual (`man zshexpn`, `man zshparam`, `man zshmisc`) to name
the language feature involved. One issue per language feature — split broad
"file X fails" findings into narrower feature issues. Worked examples:
[#11](https://github.com/z-shell/zsh-lint/issues/11) parameter-expansion
flags and operators, [#13](https://github.com/z-shell/zsh-lint/issues/13)
multi-name loops, [#15](https://github.com/z-shell/zsh-lint/issues/15)
reverse subscripts, [#16](https://github.com/z-shell/zsh-lint/issues/16)
filename-generation patterns,
[#53](https://github.com/z-shell/zsh-lint/issues/53) nested parameter
expansions. Label new issues `parser-gap` + `corpus`.

## 3. Minimize

Starting from the failing real file, delete everything unrelated until the
smallest script that still reproduces the same parse error remains. Validate
both directions:

    go run ./cmd/zsh-lint-survey <file>   # must FAIL with the same family
    zsh -n <file>                         # must pass — the gap is real Zsh

A fixture that `zsh -n` rejects is a broken script, not a parser gap; never
commit one to the native-valid survey corpus.

## 4. Promote to fixture

Add the minimized script as
`internal/survey/testdata/corpus/gap-<issue>-<slug>.zsh` with the standard
Zsh modeline and a leading comment naming the issue.
`TestMinimizedCorpus` (`internal/survey/corpus_test.go`) discovers fixtures
by scanning the corpus directory and enforces the naming contract:
`gap-<issue>-<slug>.zsh` must fail to parse, `ok-<slug>.zsh` must parse, and
any other name is rejected. There is no fixture
count assertion — adding fixtures never requires test edits
([#14](https://github.com/z-shell/zsh-lint/issues/14)); only the small
`requiredFixtures` baseline list is asserted by name.

### Native-invalid regression sources

False acceptance defects need the opposite contract: native Zsh rejects the
source and zsh-lint must also reject it. Store a minimized source as
`internal/parse/testdata/invalid-<issue>-<slug>.txt` and exercise it from a
focused parser test. Use `.txt` deliberately so the repository-wide native Zsh
syntax gate continues to require every tracked `.zsh` corpus source to be
valid.

Record the native decision with `zsh -f -n`, but never execute an invalid test
source. Parser tests read its bytes and assert the error family and original
source position. Do not add exceptions to the native Zsh syntax gate for
ordinary `.zsh` files.

## 5. Close the loop

When a front-end change (or a front-end swap,
[#17](https://github.com/z-shell/zsh-lint/issues/17)) makes a `gap-*`
fixture parse, the test fails loudly. Rename the fixture to `ok-<slug>.zsh`
so it becomes permanent regression coverage, update `requiredFixtures`, and
close the issue with a link to the survey run confirming the originating
real file now parses.

### Front-end strategy

`zsh-lint` owns its parser coverage. The mvdan/sh front end is a source of
fixes to take and test, not a dependency to wait on
([ADR-0023](https://github.com/z-shell/.github/blob/main/decisions/0023-zsh-lint-parser-front-end-strategy.md)).
This replaces the upstream-first wording in the 2026-06-12 records and
supersedes the upstream-first acceptance criterion recorded in #125 (closed;
its text is historical).

- Fix a proven valid-Zsh gap locally, under the adapter contract below, without
  conditioning on an upstream response. Prioritize by corpus evidence.
- Link an existing upstream issue as a reference. Filing new upstream issues is
  optional and never appears in acceptance criteria.
- A front-end bump is a parser behavior change. Land it with tree-shape
  assertions for every corpus fixture the release affects and a survey run
  before and after. A `gap-*` fixture that stops erroring must fail on tree
  shape, not pass vacuously: v3.14.1 turns `foreach ... end` (#214) from a parse
  error into a silent three-command tree.
- Fork the `syntax` package only when a tracked gap needs an AST node the
  upstream tree lacks and the metadata exception below cannot carry it, or
  when the adapter composition matrix becomes the bottleneck. The fork then
  replaces adapters for the constructs it covers. `repeat` (#208) was the first
  candidate; the metadata exception carries it as a `while` loop plus
  `File.RepeatLoops`, so no fork exists yet.

### Local compatibility adapters

A narrowly scoped adapter in `internal/parse` may close a proven valid-Zsh gap
without changing the selected parser dependency only when all of these hold:

- the released Zsh manual and `zsh -f -n` establish the construct's validity;
- the adapter activates for one language construct and, by default, one exact
  parser error. A construct the parser reads as an ordinary command until its
  body fails (`repeat count do ...` fails on `do`, `then`, `}` or whatever the
  body's first reserved token is) has no exact error to gate on; such an
  adapter may gate on the error position instead, at or after a site of the
  construct found by scanning the source, provided the retry is verified: the
  tree the retry produced must contain the construct's expected node at the
  site, or the adapter returns the parser error;
- the full-file retry maps every byte back to the original source, either
  by keeping the original byte length or through a source map;
- every transformed byte is restored in the typed AST before analysis;
- no mask swallows a byte that produces a `*syntax.Comment` node: the parser
  runs with `KeepComments(true)` and `internal/suppress` reads those nodes for
  directives, so a shape whose comment would have to be masked keeps the
  parser error;
- regression tests prove original AST text, diagnostic positions, suppression
  behavior, and invalid-syntax rejection; and
- the minimized fixture is `ok-*` only after the normal analyzer path passes.

If recognition or restoration is uncertain, return the parser error. Do not
use generic error suppression, recovery ASTs, or a compatibility adapter to
absorb a second untracked language feature.

### Adapter composition

Register every adapter in `adapterChain` (`internal/parse/adapter_chain.go`)
and route its masked retry through `parseWithAdapters`. Never call `parseTree`
directly for a retry, and never hand-write a list of peer adapters to compose
with.

A masked retry is an ordinary parse of a whole file. Any unrelated Zsh
construct elsewhere in that file must still parse, so an adapter that retries
through a subset of its peers makes correctness depend on which adapter happens
to run first. That defect shipped once: `parseAfterAlternateIf` named exactly
two peers, and 40 of 42 ordered pairs of adapter features failed to parse even
though each parsed alone. It surfaced as `${~ZI[BIN_DIR]}` failing in
`z-shell/zi` `zi.zsh` only because an alternate brace-form `if` appeared earlier
in the file.

Composition is permissive across distinct adapters, but it is not unbounded
within one. Most adapters mask a single occurrence of their feature per pass and
rely on re-entering themselves to reach a second occurrence, which is correct.
An adapter whose repeated masking would widen its own accepted grammar must opt
out by handing `retryExcluding(itself)` to its `*WithParser` helper. The
grouped-case adapter needs this: allowed to recurse, it masked one extra `)` per
pass and accepted `case x in (x|y))) : ;; esac`, which native Zsh rejects.

Because the survey reports only the first error per file, this class of defect
masks real gaps rather than merely reporting false ones. Fixing it immediately
exposed a further genuine gap in the same file. Re-run the discovery survey
after any adapter change and expect the reported set to shift.

`internal/parse/adapter_chain_test.go` enforces all of this: every ordered pair
of adapter features and all features together must parse; original text must be
restored; invalid Zsh must still be rejected; self-recursion must not widen the
grammar; and every snippet must genuinely require its adapter, so a snippet the
base parser already accepts cannot make its cases vacuously pass. Adding an
adapter to the chain extends that matrix automatically.

A bounded local island may use source-mapped synthetic terminators only when
the adapter verifies the original AST structure, invalid-syntax behavior, and
every source position before returning the complete tree. This exception does
not permit general source rewriting or consuming separators from the
full-file retry.

When an upstream AST has no field for a native construct, the parser result may
retain source-mapped typed syntax nodes as explicit `parse.File` metadata. This
exception requires the adapter gate and byte-preserving retry above, plus a
stable association with the owning AST node. Consumers must inspect the typed
metadata rather than recover masked source text. Anonymous-function invocation
words use this boundary because mvdan/sh v3.13.1 represents the declaration but
has no field for its invocation arguments.
The unconditional assignment operator `${name::=word}` (#216) uses it too: the
tree carries the closest typed shape, the conditional `:=` operator, and
`File.AssignAlwaysExpansions` names the expansions whose source operator is
`::=`. A second subscript `${name[a][b]}` (#215) has no index field to go to:
the retry joins both subscripts into the one index as a comma expression, and
`Parse` splits that expression at the commas whose source bytes are `][`,
leaving the first subscript in `Index` and the rest, as the parser's own typed
arithmetic nodes, in `File.SecondSubscripts`.
The expression after `,` in a flagged subscript, `${name[(r)pattern,--[^:]##]}`
(#277), needs no metadata: native Zsh reads that expression by the parameter's
type (a pattern for an associative array, arithmetic or a flagged pattern for a
plain array), which no parser can know, so an expression the parser already
reads as arithmetic keeps that reading, and only one it rejects (a bracket
expression, a leading `--`, a `^`) is retried as one literal, the shape the
parser already gives `${name[(r)a,b]}`: a `BinaryArithm` `,` whose right
operand is a `Word`.
The short form of select, `select name [in word ...] term sublist` (#212),
needs no metadata either: the tree is the `ForClause` with `Select` set that
the parser gives the `do` form, with `do` and `done` inserted through a source
map at the body's first byte and after the sublist. The body has no tree
before the retry, so a byte-preserving probe first blanks the header of every
unread site and parses through the chain; the statement at the body's first
byte, widened through the `&&`, `||`, `|` and `time` operators it is the left
operand of, is the sublist native Zsh runs, and its end is where `done` goes.
The last unread site is rewritten first, so a site whose body is another
site sees that loop with a real `done`. A `{ list }` body, an empty body, the
parenthesized list form, and a body whose last token is a closing keyword
another adapter synthesized (its rebased position does not carry the
keyword's length) keep the parser error.
The `repeat count sublist` loop (#208) has no node at all in mvdan/sh through
v3.14.1, which reads `repeat` as a command name. `resolveRepeatLoops` rewrites
each loop, after the file parses, into a `WhileClause` positioned at the
`repeat` word whose only condition is the count word, with source-mapped
`do` and `done` inserted around the body native Zsh runs (the next sublist,
a `do ... done` block or a `{ ... }` block); the sublist's last byte is
scanned back from the next statement or the enclosing closer, as the short
`if` adapter does, since a closing keyword another adapter synthesized makes
the statement's `End()` overshoot (#300); `File.RepeatLoops` names each
loop and its count so a consumer can tell it from a `while`. The loop is the
one construct whose typed node is synthesized rather than carried: a `while`
is the closest upstream shape, and the count word is kept as its condition
rather than moved into metadata so every rule that walks loop bodies sees
the body once. The condition statement is synthesized, not a command the
script runs, so the analyzer's shared walk feeds neither it nor its
`CallExpr` to any rule (`synthesizedStatements`,
`internal/analyzer/analyzer.go`) while still walking the expansions inside
the count. That skip covers only the shared walk: a rule that traverses the
tree itself from the `File` node still sees the count as a command and
today matches specific command names there.
