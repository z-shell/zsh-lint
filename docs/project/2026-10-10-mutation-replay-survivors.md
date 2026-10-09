# 2026-10-10 mutation replay survivors

Issue: [#544](https://github.com/z-shell/zsh-lint/issues/544).
Base: `7716f7c` (`origin/main`).
Runner: `.github/scripts/mutation.sh` as merged in [#601](https://github.com/z-shell/zsh-lint/pull/601), unchanged, with its default timeouts.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.

## Replays

Both replays ran in full, uncapped, with no `MUTATION_TIMEOUT` override:

    bash .github/scripts/mutation.sh --candidate 9e963299 9e963299^
    bash .github/scripts/mutation.sh --candidate 5a3b2efc 5a3b2efc^

| Candidate                                                                       | Mutants | Killed | Lived | Timed out | Invalid |
| ------------------------------------------------------------------------------- | ------- | ------ | ----- | --------- | ------- |
| [#489](https://github.com/z-shell/zsh-lint/pull/489) at `9e963299`, fixing #467 | 13      | 11     | 2     | 0         | 0       |
| [#536](https://github.com/z-shell/zsh-lint/pull/536) at `5a3b2efc`              | 264     | 180    | 72    | 5         | 7       |

The #489 numbers match the replay reported in #601.
The capped 16-mutant #536 sample reported there matches the first 16 rows of this run, 10 killed and 6 lived.
An earlier uncapped #536 run stopped before completing after 188 mutants; its rows are identical to the first 188 here.

The five timed-out mutants turn a loop increment into a decrement (`parser.go` lines 2461, 2463, 2466, 2487 and 2512 at `5a3b2efc`), so the mutant never ends; none is a survivor.

The seven invalid mutants do not build, so they are no evidence either way:

- Four turn a constant negative where Go rejects it: a slice index (`p.zshRec[:0]`) and three `byte` or `uint` values (`quote != 0`, `quote = 0`, `col = 0`).
- Three change `||` to `&&` between two comparisons of one byte with different literals, such as `c == '[' && c == ']'`, which `go vet` reports as a suspect and, failing `go test`'s build step.

Under the documented contract a mutant that does not build makes a run inconclusive, and a survivor takes precedence with exit 1.
So any fork change that compares bytes with literals or uses an unsigned constant reads inconclusive once its survivors are killed, until the runner stops generating these mutants.

## Method

Each survivor was classified on `7716f7c`, where the code of both changes is unchanged: `parser.go` lines are shifted by 45 and `redundant_separator.go` lines by 174, and `lexer.go` lines are unchanged.

- Covered now: every survivor was applied to a snapshot of `7716f7c` and run against the runner's own package set and timeout (`go test ./syntax/` in the fork, then `./internal/parse/ ./internal/survey/`, or `./internal/parse/` for #489).
  All 74 survive, so none is covered by a test added since.
- Real gap: a newline-terminated source whose parse result differs between `7716f7c` and the mutant, shown by applying the mutant and parsing the source with `syntax.NewParser(syntax.Variant(syntax.LangZsh))` or, for #489, with the scratch regression test below.
  The class says what kills it:
  - test: the witness's result on `7716f7c` agrees with `zsh -f -n`, so a regression row asserting it kills the mutant.
  - false accept: the only witness found is a source `7716f7c` accepts and `zsh -f -n` rejects, such as `print ${x['$(']}`. A row asserting today's result would pin the false accept, so the fix for that gap supplies the kill.
  - panic: the only witness goes through a source that panics on `7716f7c`, `print ${x[(r)$(echo ${[)]}`, so the fix for that panic supplies the kill.
- Equivalent: an argument from the code that no parser input can tell the mutant apart, with the reason given in the table.
  The arguments rely on facts checked in the source: the product never enables `RecoverErrors`, so the record's closer sees a parse error or a matched `]`; the scan's only caller tests `i >= 0`; and the record is always read from its start offset.
  As supporting evidence, a scratch differential harness compared each fork survivor with the unmutated parser over every subscript text of up to 7 bytes from ``$(){}[]`\'"a`` at the scan level, and about 2.6 million generated sources at the parser level.
  For the equivalent rows it found differences only at the scan level, in the -2 versus -1 returns and in texts ending in `${` or `$(`, both explained in the table; it reported a difference for every killed mutant used as a positive control.

The #489 witness is the source `; print a\`, a newline and `; print b`: `zsh -f -n` accepts it, `7716f7c` scans one redundant separator at offset 0 and parses two statements, and the mutant also reports offset 11 and parses one.
The other #489 survivor differs only when a backslash-newline ends the scanned text; both branches then leave the loop, and the state they set differently is not read after it.

## Totals

| Candidate | Lived | Equivalent | Gap, test | Gap, false accept | Gap, panic |
| --------- | ----- | ---------- | --------- | ----------------- | ---------- |
| #489      | 2     | 1          | 1         | 0                 | 0          |
| #536      | 72    | 42         | 19        | 10                | 1          |

The panic and the false accepts are parser defects on `7716f7c`, not mutation results; they were found while classifying these survivors.

## #489 survivors

| Replay row                      | Mutation         | Line on `7716f7c` | Class          | Reason or witness                                                                                                                      |
| ------------------------------- | ---------------- | ----------------- | -------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `redundant_separator.go:96:27`  | `1` to `(1 + 1)` | 270               | Equivalent     | differs only when the backslash-newline ends the scanned text, where the loop exits either way and the skipped state is not read again |
| `redundant_separator.go:100:13` | `2` to `(2 - 1)` | 274               | Real gap, test | the two lines `; print a\` and `; print b`                                                                                             |

## #536 survivors

Rows are at `5a3b2efc`; the line on `7716f7c` is where the same code is now.

| Replay row          | Mutation                                  | Line on `7716f7c` | Class                  | Reason or witness                                                                                                |
| ------------------- | ----------------------------------------- | ----------------- | ---------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `lexer.go:86:17`    | `>` to `>=`                               | 86                | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:86:19`    | `0` to `(0 - 1)`                          | 86                | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:105:19`   | `>` to `>=`                               | 105               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:105:21`   | `0` to `(0 - 1)`                          | 105               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:113:19`   | `>` to `>=`                               | 113               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:113:21`   | `0` to `(0 - 1)`                          | 113               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:153:16`   | `>` to `>=`                               | 153               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `lexer.go:153:18`   | `0` to `(0 - 1)`                          | 153               | Equivalent             | records every byte outside a subscript too; the record is read from its start offset, so only memory use changes |
| `parser.go:2419:21` | `&&` to `\|\|`                            | 2464              | Real gap, test         | `print ${x[${y["]"]},$(echo [)]}`                                                                                |
| `parser.go:2419:28` | `>` to `>=`                               | 2464              | Equivalent             | `p.w` is at least 1 once a rune is read, which always precedes a subscript                                       |
| `parser.go:2419:30` | `0` to `(0 - 1)`                          | 2464              | Equivalent             | `p.w` is at least 1 once a rune is read, which always precedes a subscript                                       |
| `parser.go:2419:32` | `&&` to `\|\|`                            | 2464              | Real gap, test         | `print ${x[${y["]"]},$(echo [)]}`                                                                                |
| `parser.go:2419:46` | `>=` to `>`                               | 2464              | Real gap, test         | a subscript starting at byte 1024, the read buffer boundary                                                      |
| `parser.go:2421:23` | `>` to `>=`                               | 2466              | Equivalent             | the first branch is always taken at depth 0, so the else branch runs only when a record is open                  |
| `parser.go:2421:25` | `0` to `(0 - 1)`                          | 2466              | Equivalent             | the first branch is always taken at depth 0, so the else branch runs only when a record is open                  |
| `parser.go:2425:19` | `1` to `(1 + 1)`                          | 2470              | Equivalent             | the closer reads `zshIndexEnd` only without a parse error, and then `index()` has just set it                    |
| `parser.go:2425:19` | `1` to `(1 - 1)`                          | 2470              | Equivalent             | the closer reads `zshIndexEnd` only without a parse error, and then `index()` has just set it                    |
| `parser.go:2427:13` | `--` to `++`                              | 2472              | Real gap, panic        | `print ${x[(r)$(echo ${y[1]} ${[)]}`                                                                             |
| `parser.go:2429:20` | `1` to `(1 + 1)`                          | 2474              | Equivalent             | the closer reads `zshIndexEnd` only without a parse error, and then `index()` has just set it                    |
| `parser.go:2429:20` | `1` to `(1 - 1)`                          | 2474              | Equivalent             | the closer reads `zshIndexEnd` only without a parse error, and then `index()` has just set it                    |
| `parser.go:2431:7`  | `p.zshRecOn == 0` to `!(p.zshRecOn == 0)` | 2476              | Real gap, test         | `print ${x[$(echo [),${y[1]}]}`                                                                                  |
| `parser.go:2431:18` | `==` to `!=`                              | 2476              | Real gap, test         | `print ${x[$(echo [),${y[1]}]}`                                                                                  |
| `parser.go:2431:21` | `0` to `(0 + 1)`                          | 2476              | Real gap, test         | `print ${x[$(echo [),${y[1]}]}`                                                                                  |
| `parser.go:2431:21` | `0` to `(0 - 1)`                          | 2476              | Equivalent             | the buffer is never truncated; the record is read from its start offset, so only memory use changes              |
| `parser.go:2432:26` | `0` to `(0 + 1)`                          | 2477              | Equivalent             | one stale byte stays below the next record's start offset                                                        |
| `parser.go:2435:19` | `\|\|` to `&&`                            | 2480              | Equivalent             | differs only after a parse error, when the scan is in bounds and a second error is dropped                       |
| `parser.go:2435:29` | `0` to `(0 - 1)`                          | 2480              | Equivalent             | without a parse error, `0 <= from <= end <= len(zshRec) - 2`; the product never enables error recovery           |
| `parser.go:2435:31` | `\|\|` to `&&`                            | 2480              | Equivalent             | without a parse error, `0 <= from <= end <= len(zshRec) - 2`; the product never enables error recovery           |
| `parser.go:2435:38` | `<` to `<=`                               | 2480              | Equivalent             | at `end == from` the record is empty and the scan reports nothing                                                |
| `parser.go:2435:45` | `\|\|` to `&&`                            | 2480              | Equivalent             | without a parse error, `0 <= from <= end <= len(zshRec) - 2`; the product never enables error recovery           |
| `parser.go:2435:52` | `>` to `>=`                               | 2480              | Equivalent             | without a parse error, `0 <= from <= end <= len(zshRec) - 2`; the product never enables error recovery           |
| `parser.go:2439:45` | `>=` to `>`                               | 2484              | Equivalent             | the scan never reports byte 0: every counted bracket follows `${`, `$(` or a backquote                           |
| `parser.go:2439:48` | `0` to `(0 + 1)`                          | 2484              | Equivalent             | the scan never reports byte 0: every counted bracket follows `${`, `$(` or a backquote                           |
| `parser.go:2457:18` | `1` to `(1 + 1)`                          | 2502              | Equivalent             | a first-bracket index of -2 is replaced exactly like -1 at the first counted bracket                             |
| `parser.go:2457:22` | `1` to `(1 + 1)`                          | 2502              | Equivalent             | a first-bracket index of -2 is replaced exactly like -1 at the first counted bracket                             |
| `parser.go:2463:15` | `<` to `<=`                               | 2508              | Real gap, false accept | ``print ${x['`']}``                                                                                              |
| `parser.go:2468:14` | `<` to `<=`                               | 2513              | Equivalent             | a bracket in a backquoted body is at index 1 or later                                                            |
| `parser.go:2468:16` | `0` to `(0 + 1)`                          | 2513              | Equivalent             | a bracket in a backquoted body is at index 1 or later                                                            |
| `parser.go:2479:9`  | `>=` to `>`                               | 2524              | Real gap, false accept | ``print ${x['`[']}``                                                                                             |
| `parser.go:2480:13` | `1` to `(1 + 1)`                          | 2525              | Equivalent             | the only caller tests `i >= 0`, so -2 acts like -1                                                               |
| `parser.go:2480:13` | `1` to `(1 - 1)`                          | 2525              | Real gap, false accept | ``print ${x['`']}``                                                                                              |
| `parser.go:2482:25` | `1` to `(1 + 1)`                          | 2527              | Equivalent             | differs only for a record ending in `${` or `$(`, which the parser rejects before the scan runs                  |
| `parser.go:2482:25` | `1` to `(1 - 1)`                          | 2527              | Real gap, test         | `print ${x[$]}`                                                                                                  |
| `parser.go:2482:27` | `<` to `<=`                               | 2527              | Real gap, test         | `print ${x[$]}`                                                                                                  |
| `parser.go:2484:13` | `2` to `(2 + 1)`                          | 2529              | Real gap, test         | `print ${x[${}$(echo [)]}`                                                                                       |
| `parser.go:2489:34` | `!=` to `==`                              | 2534              | Real gap, test         | `print ${x[1,${z:-""[}]}`                                                                                        |
| `parser.go:2490:10` | `s[i] == '\\'` to `!(s[i] == '\\')`       | 2535              | Real gap, test         | `print ${x[${z:-"a"}$(echo [)]}`                                                                                 |
| `parser.go:2490:15` | `==` to `!=`                              | 2535              | Real gap, test         | `print ${x[${z:-"a"}$(echo [)]}`                                                                                 |
| `parser.go:2491:9`  | `++` to `--`                              | 2536              | Real gap, test         | `print ${x[1,${z:-"\a"}]}`                                                                                       |
| `parser.go:2503:13` | `1` to `(1 + 1)`                          | 2548              | Equivalent             | the only caller tests `i >= 0`, so -2 acts like -1                                                               |
| `parser.go:2506:20` | `&&` to `\|\|`                            | 2551              | Real gap, test         | `print ${x[$a,$(echo [)]}`                                                                                       |
| `parser.go:2506:25` | `1` to `(1 + 1)`                          | 2551              | Equivalent             | differs only for a record ending in `${` or `$(`, which the parser rejects before the scan runs                  |
| `parser.go:2506:25` | `1` to `(1 - 1)`                          | 2551              | Real gap, test         | `print ${x[$]}`                                                                                                  |
| `parser.go:2506:27` | `<` to `<=`                               | 2551              | Real gap, test         | `print ${x[$]}`                                                                                                  |
| `parser.go:2509:13` | `2` to `(2 + 1)`                          | 2554              | Real gap, test         | `print ${x[$()$(echo [)]}`                                                                                       |
| `parser.go:2509:18` | `<` to `<=`                               | 2554              | Real gap, false accept | `print ${x['$(']}`                                                                                               |
| `parser.go:2511:40` | `1` to `(1 + 1)`                          | 2556              | Real gap, false accept | `print ${x['$(['$(echo \'\<NUL>\)]}`                                                                             |
| `parser.go:2511:40` | `1` to `(1 - 1)`                          | 2556              | Real gap, false accept | `print ${x['$("'\<NUL>\]}`                                                                                       |
| `parser.go:2511:42` | `<` to `<=`                               | 2556              | Real gap, false accept | `print ${x['$("'\<NUL>\]}`                                                                                       |
| `parser.go:2511:42` | `<` to `>=`                               | 2556              | Real gap, test         | `print ${x[1,$(echo \) [)]}`                                                                                     |
| `parser.go:2534:15` | `0` to `(0 + 1)`                          | 2579              | Real gap, false accept | `print ${x['$([']}`                                                                                              |
| `parser.go:2535:13` | `1` to `(1 + 1)`                          | 2580              | Equivalent             | the only caller tests `i >= 0`, so -2 acts like -1                                                               |
| `parser.go:2535:13` | `1` to `(1 - 1)`                          | 2580              | Real gap, false accept | `print ${x['$(']}`                                                                                               |
| `parser.go:2546:10` | `1` to `(1 + 1)`                          | 2591              | Equivalent             | the only caller tests `i >= 0`, so -2 acts like -1                                                               |
| `parser.go:2550:8`  | `<` to `<=`                               | 2595              | Equivalent             | a counted bracket is at index 2 or later, so `< 0`, `<= 0` and `< 1` agree                                       |
| `parser.go:2550:10` | `0` to `(0 + 1)`                          | 2595              | Equivalent             | a counted bracket is at index 2 or later, so `< 0`, `<= 0` and `< 1` agree                                       |
| `parser.go:2553:5`  | `c == '['` to `!(c == '[')`               | 2598              | Equivalent             | the sign of every count step flips; the count is only compared with zero                                         |
| `parser.go:2553:7`  | `==` to `!=`                              | 2598              | Equivalent             | the sign of every count step flips; the count is only compared with zero                                         |
| `parser.go:2581:25` | `&&` to `\|\|`                            | 2626              | Real gap, test         | `print ${x[$(echo [) .]}`                                                                                        |
| `parser.go:2581:39` | `>` to `>=`                               | 2626              | Equivalent             | adds writes while no record is open; the next record resets the value before reading it                          |
| `parser.go:2581:41` | `0` to `(0 - 1)`                          | 2626              | Equivalent             | adds writes while no record is open; the next record resets the value before reading it                          |
| `parser.go:2584:41` | `1` to `(1 - 1)`                          | 2629              | Real gap, false accept | ``print ${x['`']}``                                                                                              |
