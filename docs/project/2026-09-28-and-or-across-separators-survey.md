# 2026-09-28 and-or across separators survey

Issue: [#548](https://github.com/z-shell/zsh-lint/issues/548).
Base: `441a5909` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.
Trees are compared with Zsh's own deparse, as `TestStructuralOracle` does.
Manual: [Simple Commands & Pipelines](https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines).
Source: `par_sublist` in `Src/parse.c` at the `zsh-5.9.2` tag.

## Change

After a `&&` or `||`, Zsh's `par_sublist` skips every separator token before it reads the right operand, and `;` and a newline both lex as a separator.
So `false && ; print b` is the one sublist `false && print b`, and running it prints nothing.
The fork read the operator as having no right operand, and the dangling-operator adapter (#331) then masked it to `;`, which split the sublist into two statements: a rule walking the tree saw `print b` as unconditional.

`getStmt` in the fork now steps over the `;` and newline separators after the operator and reads the next statement as its right operand.
When a reserved word that closes a list (`}`, `then`, `elif`, `else`, `fi`, `do`, `done`, `esac`, `end`) or a token that cannot start a statement follows the separators, the operator still dangles and the fork reports it at the operator as before, so the adapter handles it unchanged.
Without a `;` nothing changes: `print a &&` then a newline and `print b` was already one sublist.
Bash, POSIX and mksh keep upstream's error.

`ok-dangling-and-or.zsh` leaves `structuralOracleKnownDifferences`: its printed tree is now the program Zsh reads.

## Rows

All are top-level probes unless the source says otherwise.

| Row | Source                                  | Native | Base                 | Fixed               |
| --- | --------------------------------------- | ------ | -------------------- | ------------------- |
| 1   | `false &&` `;` `print b` (three lines)  | accept | accept, 2 statements | accept, one sublist |
| 2   | `false && ; print b`                    | accept | accept, 2 statements | accept, one sublist |
| 3   | `false && ; true \|\| ; print c`        | accept | accept, 2 statements | accept, one sublist |
| 4   | `f() { false &&` `;` `print b` `}`      | accept | accept, 2 statements | accept, one sublist |
| 5   | `echo "$( false && ; print b )"`        | accept | reject               | accept              |
| 6   | `while false && ; { print b; }; do ...` | accept | reject               | accept              |
| 7   | `false && ;`                            | accept | accept               | accept              |
| 8   | `{ false && ; }`                        | accept | accept               | accept              |
| 9   | `false && ; ]]`                         | accept | reject               | reject              |
| 10  | `echo "$( false && ; )"`                | accept | reject               | reject              |
| 11  | `false && ; print b` then `}`           | reject | reject at 1:10       | reject at 2:1       |

Rows 5 and 6 were rejected because the adapter's mask left a statement the base could not read; the fork reads them directly.
Rows 9 and 10 are known gaps left as they were: a lone `]]` in command position is valid Zsh (`command not found` when run), and the adapter does not reach a dangling operator inside a double-quoted substitution.
In row 11 the error moves to the stray `}`, where Zsh reports it.

## Verification

- Corpus: `zsh-lint-survey -compare <base> -native internal/survey/testdata/corpus/*.zsh`: 103 files, all unchanged.
- Workspace: the same comparison over 1208 Zsh files under `repos/` of the Z-Shell workspace and the other Z-Shell checkouts, including `zpmod`'s vendored Zsh trees: all unchanged.
- Probe grid: 4368 generated files crossing the operator (`&&`, `||`, `|`, `|&`), 12 spellings between the operator and what follows (`;` forms, newlines, a comment, a line continuation, `;;`, `;&`, `;|`, `&`, nothing), 31 followers (commands, compound commands, function definitions, a here-document, reserved closers, operators, end of input) and 13 enclosing contexts.
  Judged per file against native Zsh and the base: 30 fixed (rows 5 and 6 above), no introduced false accept or regression, no panic.
  The 8 files both builds accept and `zsh -f -n` rejects are `! print b` operands, where `zsh -f -n` exits 1 without a message; they run without an error.
  The 467 files both builds reject are the `|` and `|&` pipe across a separator (427, a separate gap), a `]]` operand (16), row 10 (16), a second operator in a brace-form `while` condition (6, which Zsh does not run as the loop's body either), and a `do` straight after the operator in a `while` condition (2).
- Parse cost (`-trace-parses`): corpus 624 parses before and 622 after, adapter depth unchanged; `ok-dangling-and-or.zsh` 9 before and 7 after.
- Hand mutants of the fork change (the mutation script does not reach the fork module): 17 of 17 killed, covering the gate, its dialect and `;` conditions, both constant returns of the new function, the separator loop, and each reserved word.
- `go build ./...`, `go vet ./...`, `go test ./...`, the fork's `go test ./syntax/`, and `golangci-lint` v2.12.2 all pass.
