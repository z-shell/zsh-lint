# 2026-09-28 pipe across separators survey

Issue: [#553](https://github.com/z-shell/zsh-lint/issues/553).
Base: `a92ad17` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.
Trees are compared with Zsh's own deparse, as `TestStructuralOracle` does.
Manual: [Simple Commands & Pipelines](https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines).
Source: `par_pline` in `Src/parse.c` at the `zsh-5.9.2` tag.

## Change

After a `|` or `|&`, Zsh's `par_pline` skips every separator token before it reads the next command, and `;` and a newline both lex as a separator.
So `print a | ; cat` is the one pipeline `print a | cat`, which Zsh deparses that way and which prints `a` when it runs.
The fork reported the pipe as having no right operand.

The pipe loop in `gotStmtPipe` now uses the #548 helper, `zshSkipOperandSeparators`, to step over the `;` and newline separators after the operator, and reads the next command as its right operand.
Unlike `&&` and `||`, a pipe never dangles in Zsh, so when a reserved word that closes a list or a stop token follows the separators, the error stays at the operator as before.
Without a `;` nothing changes: `print a |` then a newline and `cat` was already one pipeline.
Bash, POSIX and mksh keep upstream's error.

## Rows

All are top-level probes unless the source says otherwise.

| Row | Source                               | Native | Base   | Fixed              |
| --- | ------------------------------------ | ------ | ------ | ------------------ |
| 1   | `print a \| ; cat`                   | accept | reject | accept, a pipeline |
| 2   | `print a \|` `;` `cat` (three lines) | accept | reject | accept, a pipeline |
| 3   | `print a \|& ; cat`                  | accept | reject | accept, a pipeline |
| 4   | `f() { print a \| ; cat }`           | accept | reject | accept             |
| 5   | `echo "$(print a \| ; cat)"`         | accept | reject | accept             |
| 6   | `print a \| ;`                       | reject | reject | reject at 1:9      |
| 7   | `{ print a \| ; }`                   | reject | reject | reject at 1:11     |
| 8   | `print a \| ; ]]`                    | accept | reject | reject             |
| 9   | `print a \| ; coproc cat`            | reject | reject | accept             |
| 10  | `print a \| ; for }`                 | reject | reject | accept             |

Row 8 is the known gap [#554](https://github.com/z-shell/zsh-lint/issues/554): a lone `]]` in command position is valid Zsh.
Rows 9 and 10 are false accepts this change inherits rather than introduces.
`main` already accepts both without the `;` (`print a | coproc cat`, `print a | for }`), and the fork now reads the pipe's operand as Zsh does, so the separator no longer hides them.
`coproc` after a pipe is part of [#282](https://github.com/z-shell/zsh-lint/issues/282), which has a comment recording the pipe position, and a first loop name that is not an identifier is [#557](https://github.com/z-shell/zsh-lint/issues/557).

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 104 files, 103 unchanged, and `ok-pipe-across-separators.zsh` fixed.
- Workspace: the same comparison over 1333 Zsh files under `repos/` of the Z-Shell workspace, including `zpmod`'s vendored Zsh trees: all unchanged.
- Probe grid: 30720 generated files crossing the operator (`|`, `|&`), 8 spellings between the operator and what follows (`;` forms, newlines, a blank line, a comment, nothing), 44 followers (commands, prefixed and compound commands, function definitions, a here-document, reserved closers, operators, end of input), 6 left operands and 21 enclosing contexts.
  Judged per file against native Zsh and the base: 10584 fixed, no regression, no panic or hang.
  608 files are rows 9 and 10 above: 560 `coproc` and 48 `select }` operands.
  Every file whose source holds a pipe followed by separators was also compared with the same file with those separators removed: the fixed binary's verdict on each of the 27496 files equals the base binary's verdict on its separator-free twin.
  The 160 files both builds accept and `zsh -f -n` rejects are `coproc` (144) and `select }` (16) with no separator.
  The 672 files both builds reject are a `]]` operand (640, #554) and a here-document whose terminator line also holds the brace-form `while` closer (32), which Zsh reads to the end of the file.
- Parse cost (`-trace-parses`): corpus 624 parses before and 623 after, adapter depth unchanged; `ok-pipe-across-separators.zsh` 2 before and 1 after.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 8 of 8 killed, covering the gate forced on and off, its dialect and `;` conditions, the helper's result negated and ignored, and each branch's operand.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
