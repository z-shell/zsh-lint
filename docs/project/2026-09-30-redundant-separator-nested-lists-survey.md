# 2026-09-30 redundant separator in nested lists survey

Issue: [#569](https://github.com/z-shell/zsh-lint/issues/569).
Base: `b32ef64` (`origin/main`), built with `go build` into a scratch binary.
Oracle: Zsh 5.9.2, newline-terminated files checked with `zsh -f -n FILE`; empty stderr means valid.
Manual: [Simple Commands & Pipelines](https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines).

## Change

A `;` in command position ends an empty sublist, which Zsh reads as a no-op (#332).
The redundant-separator adapter found such a `;` at the top level, after a statement and in an unquoted `$( )`, but its scanner stepped over a double-quoted string and a backquoted command whole, so a `;` opening the list of `"$( ; print a )"` or `` `; print a` `` was rejected.

`scanRedundantSeparatorSites` now scans the body of a backquoted command, and the command substitutions and backquoted commands inside a double-quoted string, as lists of their own.
Arithmetic and parameter expansions inside the string are stepped over with the shared `double_quote.go` scanners, not entered.
Where a shared scanner cannot tell an extent (an unclosed string or backquote, a substitution holding `case` or a here-document, arithmetic holding `$`), the string reports no site and keeps its verdict on `main`.
A nested body holding `$'` or `<<` reports no site either: the scanner does not read `$'...'` escapes or here-document bodies as Zsh does, and a site there would blank a byte of a string.
The scanner also no longer reads the `&` or `|` of a redirection operator (`>&`, `<&`, `>|`, `>&|`) as a list operator, so the `;` in `print a >& ; print b` is not a site.
Before this change that was a false accept whenever a file also held a redundant `;` the adapter could fix; with nested sites it would have been reachable from any quoted or backquoted `$( ; )`.
The nested sites are masked in their own pass, when the parser reports one of them, without the top-level sites.
The top-level scan still misreads a `;` in a glob group, a `[[ ]]` pattern or after `> (`, and inside `${...}` and `$'...'` ([#572](https://github.com/z-shell/zsh-lint/issues/572)); masking both sets together let a fixed substitution reach those.
When the parser reports a top-level `;`, the nested sites are tried on top of the existing one-pass top-level mask, and dropped when they do not hold, which leaves base's own retry.
A nested body is scanned with the same scanner, so it can take a `;` inside a glob group, a quoted string, `${...}` or a comment for a site.
After each nested retry the tree is checked: a blank that lands inside a literal, a single-quoted string or a comment is text, its byte is put back, and the source is parsed once more; a retry that still holds a blank inside text is refused.
A `;` that really ends an empty sublist owns no node, so this keeps every word, string and comment as written.
A blank can also land between two operands rather than inside text: after `||`, `&&` or `(` inside `[[ ]]`, inside `(( ))` or `$(( ))`, or among a case arm's patterns, where the masked source still parses.
So every nested site still blanked must also lie in a gap of a statement list: the smallest node holding it must be a command substitution, process substitution, brace group, subshell, `if`, `while` or `until`, `for`, a `&&`, `||` or pipe list, or a case arm past the `)` that closes its patterns.
No adapter, gate or existing test changed.

## Rows

All are top-level probes unless the source says otherwise.

| Row | Source                                    | Native | Base   | Fixed                      |
| --- | ----------------------------------------- | ------ | ------ | -------------------------- |
| 1   | `echo "$( ; print a )"`                   | accept | reject | accept                     |
| 2   | `x="$( ; print a )"; print $x`            | accept | reject | accept                     |
| 3   | `f() { echo "$( ; print a )"; }; f`       | accept | reject | accept                     |
| 4   | `echo "$(echo "$( ; print a )")"`         | accept | reject | accept                     |
| 5   | ``echo `; print a` ``                     | accept | reject | accept                     |
| 6   | ``echo "`; print a`"``                    | accept | reject | accept                     |
| 7   | `echo "$(print a; ; print b)"`            | accept | reject | accept                     |
| 8   | `echo "$(true \| ; )"`                    | reject | reject | reject                     |
| 9   | `echo "$(if true; ; )"`                   | reject | reject | reject                     |
| 10  | `( ; print a )`                           | accept | reject | reject                     |
| 11  | `{ ; print a }`                           | accept | reject | reject                     |
| 12  | `case x in x) ; print a ;; esac`          | accept | reject | reject                     |
| 13  | `echo "${x:-$( ; print a )}"`             | accept | reject | reject                     |
| 14  | `echo "$( ; )"; print a >& ; print b`     | reject | reject | reject                     |
| 15  | `; print a >& ; print b`                  | reject | accept | reject                     |
| 16  | `echo "$( ; )"; print a(;)`               | reject | reject | reject                     |
| 17  | `echo "$( ; )"; print ${x:-` newline `;}` | accept | reject | accept, `${...}` text kept |
| 18  | ``echo `; print a(;)` ``                  | reject | reject | reject                     |
| 19  | `; echo "$( [[ a == (;) ]] )"`            | reject | reject | reject                     |
| 20  | `echo "$( ; print ${x:-` newline `;} )"`  | accept | reject | accept, `${...}` text kept |
| 21  | `; echo "$( print a # (;` newline `)"`    | accept | accept | accept, comment kept       |

Rows 10 and 11 stay open under #569: they fail with the opener's own error before the adapter sees the `;`, and `gap-569-leading-separator-after-opener.zsh` records them.
Row 12 is [#571](https://github.com/z-shell/zsh-lint/issues/571), and row 13 is a substitution nested in a parameter expansion inside double quotes; both fail identically on base and fixed.
Row 14 is the shape the redirection rule keeps rejected, and row 15 is a base false accept it removes.
Rows 16 to 21 are the review shapes: a #572-class `;` beside or inside a nested body keeps base's verdict for it, and every word and comment keeps its text.
`; print a(;)` itself stays a base false accept, [#572](https://github.com/z-shell/zsh-lint/issues/572), and so does `; echo "$( ; )"; print a(;)`, whose twin without the nested `;` base also accepts.

## Verification

- `.github/scripts/verify-parser-change.sh --bodies bodies.txt --rows rows.txt`: build, vet, tests, the fork's tests, `golangci-lint` v2.12.2 and mutation all pass.
- Corpus: 106 files, 105 unchanged, `ok-redundant-separator-nested.zsh` fixed.
- Probe grid (8 bodies in every `zsh-lint-probe` context): 208 files, 203 unchanged, 5 fixed, no regression and no false accept.
- Row grid (70 sources): 45 unchanged, 23 fixed, 2 rejected, no regression and no false accept.
  The two rejected rows, `; print a >& ; print b` and `echo $( ; ); print a >& ; print b`, are false accepts on base that the redirection rule removes.
- Four independent review grids (533 rows, including a fixed nested `;` beside or inside a body with an invalid `;` in a glob group, `[[ ]]` pattern or operand, arithmetic, process substitution, redirection, `${...}`, `$'...'`, a quoted string or a comment), judged native, base, fixed: no regression, 3 base false accepts removed, the pre-existing false accepts left.
  Three rows change from rejected to accepted although top-level `zsh -f -n` rejects them: `echo "$( ; )"; print ${(;)x}`, its backquoted twin and `echo "$( ; )"; print $x[(r);]`.
  `zsh -f -n` reports those only through expansion at the top level (`error in flags`, section 3 of `parser-gap-workflow.md`); the same text inside a function body passes it, and base already accepts `print ${(;)x}` alone.
  Two more, `; echo "$( ; )"; print a(;)` and its backquoted twin, are accepted because base accepts the same file without the nested `;` (#572).
  One more, `echo "$( ; nocorrect ; )"`, is accepted because base accepts `echo "$(   nocorrect ; )"` with the same tree: a lone `nocorrect` is a base false accept ([#395](https://github.com/z-shell/zsh-lint/issues/395)).
- Stripped-twin grid (352 generated rows: each of four fixed nested bodies beside 22 other constructs, with and without a leading `;`, in both orders): the fixed build never accepts a row that base rejects with its nested `;` blanked, and every text-bearing node keeps its bytes.
  Where it rejects a row that such a twin passes, the twin passes only through #572: a `;` after `>&` or inside a glob group that base's top-level mask blanks, or a `$'...'` string beside the body whose `;` base's top-level mask blanks, changing the string (`print $'a\'; ;'`); the fixed build keeps base's rejection of the row.
- Redirection grid (54 rows: 18 operators including `>>`, `&>`, `>!`, `<>`, `>&-`, `&|`, `&!` and process substitution, each alone, after a leading `;` and after a fixed `$( ; )`): no introduced false accept, no regression.
- Tree text: a generated grid of 261 rows placing quoted strings, `${...}`, glob groups and comments holding `;` in nested bodies, and tests that compare every tree with the tree of the source with only its real sites blanked.
- List gaps: a `;` in each list-owning construct (pipeline and `&&` lists, subshell, brace group, `if`, `elif`, `else`, `while`, `until`, `for`, process substitution, case arm body) stays fixed; one after `||`, `&&` or `(` in `[[ ]]`, inside `(( ))` and `$(( ))`, or in a case pattern list keeps base's rejection.
- Workspace: 1367 Zsh files under `repos/` of the Z-Shell workspace, including `zpmod`'s vendored Zsh trees: all unchanged.
- Mutation (`mutation.sh`): every decided mutant killed; the backquote `case` line reads as not covered, as `case` expressions do.
- Parse cost (`-trace-parses` over the corpus): 625 parses before and 626 after, the difference being the fixed `ok-redundant-separator-nested.zsh`, which now takes the adapter's one retry (2 parses, adapter depth 1); a test pins one retry for a file with several nested sites.
  A nested retry that finds text parses once more, and a failed nested attempt on the top-level path adds one parse before base's own retry.
- Hand mutants of the change (61): 57 killed.
  One starts the nested scan one byte early on the `(`, which the scan reads as an opener and the offset shift cancels.
  One changes the redirection rule's `index == 0` bound, which only guards reading `src[-1]`.
  Two remove a restore check that no measured source reaches: skipping the check after the second parse, and not restoring a single-quoted string, whose `;` the scanner already skips as a quote; a 261-row generated grid gives identical verdicts and trees for both.
  No source tells any of the four apart.
