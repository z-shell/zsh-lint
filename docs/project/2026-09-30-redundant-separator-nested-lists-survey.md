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
No adapter, gate or existing test changed.

## Rows

All are top-level probes unless the source says otherwise.

| Row | Source                                | Native | Base   | Fixed  |
| --- | ------------------------------------- | ------ | ------ | ------ |
| 1   | `echo "$( ; print a )"`               | accept | reject | accept |
| 2   | `x="$( ; print a )"; print $x`        | accept | reject | accept |
| 3   | `f() { echo "$( ; print a )"; }; f`   | accept | reject | accept |
| 4   | `echo "$(echo "$( ; print a )")"`     | accept | reject | accept |
| 5   | ``echo `; print a` ``                 | accept | reject | accept |
| 6   | ``echo "`; print a`"``                | accept | reject | accept |
| 7   | `echo "$(print a; ; print b)"`        | accept | reject | accept |
| 8   | `echo "$(true \| ; )"`                | reject | reject | reject |
| 9   | `echo "$(if true; ; )"`               | reject | reject | reject |
| 10  | `( ; print a )`                       | accept | reject | reject |
| 11  | `{ ; print a }`                       | accept | reject | reject |
| 12  | `case x in x) ; print a ;; esac`      | accept | reject | reject |
| 13  | `echo "${x:-$( ; print a )}"`         | accept | reject | reject |
| 14  | `echo "$( ; )"; print a >& ; print b` | reject | reject | reject |
| 15  | `; print a >& ; print b`              | reject | accept | reject |

Rows 10 and 11 stay open under #569: they fail with the opener's own error before the adapter sees the `;`, and `gap-569-leading-separator-after-opener.zsh` records them.
Row 12 is [#571](https://github.com/z-shell/zsh-lint/issues/571), and row 13 is a substitution nested in a parameter expansion inside double quotes; both fail identically on base and fixed.
Row 14 is the shape the redirection rule keeps rejected, and row 15 is a base false accept it removes.

## Verification

- `.github/scripts/verify-parser-change.sh --bodies bodies.txt --rows rows.txt`: build, vet, tests, the fork's tests, `golangci-lint` v2.12.2 and mutation all pass.
- Corpus: 106 files, 105 unchanged, `ok-redundant-separator-nested.zsh` fixed.
- Probe grid (8 bodies in every `zsh-lint-probe` context): 208 files, 203 unchanged, 5 fixed, no regression and no false accept.
- Row grid (63 sources, 20 of them native-invalid): 39 unchanged, 22 fixed, 2 rejected, no regression and no false accept.
  The two rejected rows, `; print a >& ; print b` and `echo $( ; ); print a >& ; print b`, are false accepts on base that the redirection rule removes.
- Independent review grid (90 rows, including pairs of a fixed nested `;` with an invalid `;` elsewhere in the file, judged native, base, fixed): 28 fixed, 3 base false accepts removed, 1 pre-existing false accept left (`echo "$[ ; ]"`), no regression and no introduced false accept.
- Workspace: 1367 Zsh files under `repos/` of the Z-Shell workspace, including `zpmod`'s vendored Zsh trees: all unchanged.
- Mutation (`mutation.sh`): every decided mutant killed; the backquote `case` line reads as not covered, as `case` expressions do.
- Parse cost (`-trace-parses` over the corpus): 625 parses before and 626 after, the difference being the fixed `ok-redundant-separator-nested.zsh`, which now takes the adapter's one retry (2 parses, adapter depth 1).
- Hand mutants of the new scanner (24): 22 killed.
  One starts the nested scan one byte early on the `(`, which the scan reads as an opener and the offset shift cancels.
  The other changes the redirection rule's `index == 0` bound, which only guards reading `src[-1]`.
  No source tells either apart.
