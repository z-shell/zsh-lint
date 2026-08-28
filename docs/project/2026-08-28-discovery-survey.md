# Organization Discovery Survey, 2026-08-28

Scope: parser coverage for Z-Shell repositories that are **not** in the strict
reference corpus (`corpus.md`). This is a discovery run under
[parser-gap-workflow.md](parser-gap-workflow.md) step 1, not a gate result.

The strict corpus covers 6 repositories and 20 files. The three repositories
below hold the largest Zsh sources in the organization and contribute nothing
to it, so parser gaps in them are invisible to the current gate.

## Run inputs

| Repository      | Revision   | Branch surveyed            |
| --------------- | ---------- | -------------------------- |
| `z-shell/zi`    | `2d52795b` | `feature-553`              |
| `z-shell/zpmod` | `1b35bcb4` | `code/issue-107-xdg-paths` |
| `z-shell/zunit` | `10b1e58f` | `main`                     |

`zi` and `zpmod` were surveyed at the meta-workspace's checked-out development
branches, not `main`. Re-run against `main` before treating any count here as a
released-state baseline.

Tooling: `zsh-lint` `next` at `901f3f7`, Go 1.27.0, Zsh 5.9.2. File set is
`*.zsh`, `zshrc`, and `zshenv`, with `.git` pruned.

## Parser result

| Repository | Files | Parsed | Failed |
| ---------- | ----- | ------ | ------ |
| `zpmod`    | 58    | 53     | 5      |
| `zi`       | 28    | 21     | 7      |
| `zunit`    | 15    | 14     | 1      |
| **Total**  | 101   | 88     | 13     |

Of the 13 failures, 11 are genuine parser gaps (`zsh -f -n` accepts the source)
and 2 are correctly rejected by both native Zsh and zsh-lint.

### Not parser gaps

`zpmod/tests/command/zpmod_bundle_build.zsh` and
`zpmod/tests/command/zpmod_compaudit_cache.zsh` use the ZUnit `TEST "name" { }`
DSL, which is not plain Zsh. Native Zsh reports `parse error near '}'` for both.
They must be excluded from any native-valid corpus rather than tracked as gaps.

## Gap families

Each construct below was minimized from the failing real file until only the
reproducing feature remained, then validated in both directions: `zsh -f -n`
accepts it and `cmd/zsh-lint-survey` still fails with the same error family.
The survey reports only the first error per file, so a single repository may
hold further gaps masked behind these.

### A. Assignment inside parameter expansion

    print -r -- ${${prev::=abc}:+set}

Error: `` `=` must follow a name ``. Manual: `zshexpn`, `${name::=word}`.
Real sites: `zi/lib/zsh/additional.zsh:13`, `zi/lib/zsh/side.zsh:314`. Both zi
sites reduce to this one family.

### B. Brace block terminated without a separator

    { typeset -g COLS="$(tput cols)" } 2>/dev/null

Error: `` reached EOF without matching `{` with `}` ``. Manual: `zshmisc`,
complex commands. Real site: `zi/lib/zsh/git-process-output.zsh:11`. The
redirect is incidental; the bare `{ cmd }` form fails on its own.

### C. Alternation inside a backreference test pattern

    while [[ $buf = (#b)[^"{}[]\\\"'":,]#(([\"{[]}\\\"'":,])|[\\](*))(*) ]]; do

Error: `` not a valid test operator: `|` ``. Manual: `zshexpn`, filename
generation and `(#b)` backreferences. Real site: `zi/lib/zsh/install.zsh:22`.

### D. Exclusion operator inside a glob qualifier list

    reply=( /tmp/**/_[^_.]*~*(*.zwc|*.md)(DN) )

Error: `` `^` must follow an expression ``. Manual: `zshexpn`, `~` exclusion
and `[^...]` character classes. Real site: `zi/lib/zsh/autoload.zsh:471`.

### E. `foreach` loop form

    foreach item (one)
    print $item
    end

Error: `` a command can only contain words and redirects; encountered `(` ``.
Manual: `zshmisc`, alternate loop forms. Real site:
`zi/tests/fixtures/public-contract/foreach/zi.zsh:15`.

### F. GLOB_SUBST `${~name}` after an alternate brace-form `if`

    if (( 1 )) { x=1 }
    p=${~q}

Error: `` not a valid parameter expansion operator: `~` ``. Manual: `zshexpn`,
GLOB_SUBST `${~spec}`. Real site: `zi/zi.zsh:48`.

This one is different in kind from the others and is the most important finding
in this run. `${~q}` on its own parses correctly, and the same statement after a
classic `if ...; then ...; fi` also parses correctly. The failure appears only
when an alternate brace-form `if` precedes it in the same file:

    p=${~q}                          # parses
    if (( 1 )); then x=1; fi         # parses
    p=${~q}
    if (( 1 )) { x=1 }               # FAILS
    p=${~q}

So this is not a missing language feature. It is an interaction defect in the
existing alternate brace-form `if` compatibility adapter
(`ok-alternate-if-brace.zsh`), which appears to leave state that corrupts
parsing of a later `${~...}`. Isolated by line-level delta debugging over
`zi.zsh`, which reduced 48 lines to exactly these two.

This violates the adapter contract in `parser-gap-workflow.md`: an adapter must
activate for one exact error and one construct, and must restore every
transformed byte. It should be filed as an adapter-correctness bug rather than a
`parser-gap`, and it should be fixed before further adapters are added, since
the same class of latent interaction may already be masking other results.

### G. Anonymous function with a process-substitution redirect

    () {
      print -r -- "$1"
    } <(print -r -- 'x=1')

Error: `statements must be separated by &, ; or a newline`. Manual:
`zshmisc`, anonymous functions and process substitution. Real site:
`zpmod/tests/builtin/process_substitution_source.zsh:35`.

### H. `do;` with a leading separator

    freload() { while (( $# )); do; unfunction $1; shift; done }

Error: `` `while` statement must end with `done` ``. Manual: `zshmisc`.
Real site: `zpmod/vendor/zsh/StartupFiles/zshrc:45`. This is upstream Zsh's own
shipped startup file.

### I. Subscript search with a numeric-range pattern

    integer argi=${argv[(i)-<->]}

Error: `` `-` must be followed by an expression ``. Manual: `zshparam`,
subscript flags; `zshexpn`, `<->` numeric globbing. Real site:
`zpmod/vendor/zsh/Test/ztst.zsh:93`, upstream Zsh's test harness. Related to
the closed reverse-subscript work (#15) but distinct: the pattern is `<->`
inside an `(i)` search, and it is preceded by `-`.

### J. Arithmetic subscript range containing a nested subscript search

    line=(a b); testname="${line[(( ${line[(i)[\']]}+1 )),(( ${line[(I)[\']]}-1 ))]}"

Error: `` `[` must follow a name like a[i] ``. Manual: `zshparam`, subscript
ranges. Real site: `zunit/src/commands/run.zsh:268`.

## Recommended handling

Families A through E and G through J are one `parser-gap` + `corpus` issue each,
per the workflow's one-feature-per-issue rule. Minimized sources become
`gap-<issue>-<slug>.zsh` fixtures once the issue numbers exist.

Family F is not a parser gap and must not be filed as one. It is a correctness
bug in an already-shipped adapter and should be prioritized above the new
feature work, because an adapter that corrupts later parsing can produce both
false failures and, more dangerously, masked findings anywhere in the corpus.

Do **not** add these repositories to `corpus-paths.txt` yet. The strict gate
requires zero errors and zero warnings, and these sources currently carry 11
parse errors plus 213 warning-level diagnostics (dominated by
`quoting/unquoted-var` and `plugin/zero-handling`). Promotion follows the same
sequence used on 2026-08-16: close the parser gaps, remediate consumer findings
in the owning repository, then promote and re-run the complete gate.

## Analyzer load, non-gating

Recorded so consumer remediation can be sized. Excludes `vendor/`.

| Repository | Files | Errors | Warnings | Infos | Hints |
| ---------- | ----- | ------ | -------- | ----- | ----- |
| `zi`       | 28    | 7      | 20       | 0     | 0     |
| `zpmod`    | 54    | 3      | 164      | 5     | 11    |
| `zunit`    | 15    | 1      | 29       | 3     | 72    |

Error counts are the `parse/error` diagnostics from the gaps above, not
independent semantic findings.
