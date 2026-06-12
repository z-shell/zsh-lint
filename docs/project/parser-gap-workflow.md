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
commit one.

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

## 5. Close the loop

When a front-end change (or a front-end swap,
[#17](https://github.com/z-shell/zsh-lint/issues/17)) makes a `gap-*`
fixture parse, the test fails loudly. Rename the fixture to `ok-<slug>.zsh`
so it becomes permanent regression coverage, update `requiredFixtures`, and
close the issue with a link to the survey run confirming the originating
real file now parses.
