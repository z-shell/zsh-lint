# mvdan.cc/sh fork

This directory is a fork of [`mvdan.cc/sh/v3`](https://github.com/mvdan/sh) at `v3.14.1`, kept under its upstream BSD-3-Clause license (`LICENSE`), as ADR-0030 in z-shell/.github decides. `go.mod` at the repository root replaces the module with this directory, so import paths are unchanged.

Only the packages zsh-lint builds are kept: `syntax`, `expand`, `pattern`, `fileutil`, and `internal`. Upstream's tests for them are kept and run in Go CI; they must keep passing.

## Local changes

Every change is Zsh-only, behind `LangZsh`, and listed here with the zsh-lint issue that made it.

| Change | Files | Issue |
| --- | --- | --- |
| Rejection of a lone `-` condition (`condition expected: -`), and rejection of `!` or leading `(` in the right operand of `<` and `>`, matching native Zsh condlex and par_cond_2 | `syntax/parser.go`, `syntax/module_test.go` | [#512](https://github.com/z-shell/zsh-lint/issues/512) |
| Module-defined prefix and infix conditions, multi-operand dash conditions, and lone unary-name string tests, with a Zsh-only `ModuleTest` node, positions, walk, printer and typedjson support; in their operands a glob group nests bare parentheses and numeric globs, and `;`, `&` or a bare `<`/`>` inside the group is an error, as Zsh's word lexer reads it | `syntax/parser.go`, `syntax/nodes.go`, `syntax/walk.go`, `syntax/printer.go`, `syntax/typedjson/`, `syntax/module_test.go` | [#484](https://github.com/z-shell/zsh-lint/issues/484) |
| Brace-form `if`/`elif`/`else` and `while`/`until` bodies (Alternate Forms For Complex Commands) | `syntax/parser.go` | [#446](https://github.com/z-shell/zsh-lint/issues/446) |
| A `'` in the word of a double-quoted `${...}` is text, not a single-quote opener | `syntax/parser.go`, `syntax/lexer.go` | [#400](https://github.com/z-shell/zsh-lint/issues/400) |
| In a glob group or glob qualifier, a quoted `)` and the `)` closing a command substitution do not end the group | `syntax/parser.go` | [#439](https://github.com/z-shell/zsh-lint/issues/439) |
| A case pattern whose leading group is glued to more pattern text, `(x)y)`, and a leading `((`, as in `((x\|y)\|z)` | `syntax/parser.go` | [#452](https://github.com/z-shell/zsh-lint/issues/452) |
| A case pattern with an empty alternative, such as `(\|a)`, `(a\|)`, `\|a)`, `a\|)`, `\|a\|)`, `(a\|\|b)`, `(\|)`, `\|\|)`, and `(\|a\|)` | `syntax/parser.go`, `syntax/parser_test.go` | [#396](https://github.com/z-shell/zsh-lint/issues/396) |
| The alternate and short forms of `for` and `select` (several names, a `( word ... )` list, a `{ list }` or one-sublist body, an empty body), the short forms of `if`, `while` and `until`, `else` reserved in command position, and a `ZshEnd` position on `ForClause`, `IfClause` and `WhileClause` so `End()` is exact for those forms | `syntax/parser.go`, `syntax/nodes.go`, `syntax/parser_test.go` | [#459](https://github.com/z-shell/zsh-lint/issues/459) |
| The csh-style `foreach ... end` loop (one or more names, `( word ... )` or `in word ...`, `list end`, `{ list }` or `do list done`), and `end` reserved in command position | `syntax/parser.go`, `syntax/nodes.go`, `syntax/parser_test.go` | [#214](https://github.com/z-shell/zsh-lint/issues/214) |
| A `RepeatClause` node for `repeat count` loops (`do list done`, `{ list }` or one sublist, as `for` reads its body), with walk, printer and `typedjson` support | `syntax/nodes.go`, `syntax/parser.go`, `syntax/walk.go`, `syntax/printer.go`, `syntax/typedjson/json.go` | [#281](https://github.com/z-shell/zsh-lint/issues/281) |
| Function definitions whose name is any word, holding a parameter expansion, quote or command substitution (`word()` and `function word ...`), and a name ending in a `}` that closes no `{` of the name rejected as zsh does | `syntax/parser.go`, `syntax/parser_test.go` | [#234](https://github.com/z-shell/zsh-lint/issues/234) |
| Semicolons and newlines between the case word and `in` (`case word; in`) | `syntax/parser.go`, `syntax/parser_test.go` | [#485](https://github.com/z-shell/zsh-lint/issues/485) |
| After an assignment prefix, `!`, `[[`, `{`, `(`, `time`, `coproc`, `if`, `case`, `while`, `until`, `for`, `select`, `foreach` and `function` are rejected, and a closing reserved word (`then`, `fi`, `do`, `done`, `esac`, `end`, ...) ends the command, as Zsh keeps command position after the prefix | `syntax/parser.go`, `syntax/parser_test.go` | [#278](https://github.com/z-shell/zsh-lint/issues/278) |
| A `function` keyword definition whose body is missing or is itself a bodyless or keyword definition defines each name with an empty body (`FuncDecl.Body == nil`), with `ParensEnd` for `End()`, printer, and walk support | `syntax/nodes.go`, `syntax/parser.go`, `syntax/printer.go`, `syntax/walk.go`, `syntax/parser_test.go` | [#479](https://github.com/z-shell/zsh-lint/issues/479) |
| A subscripted or array assignment before a command word (`a[2]=x cmd`, `a=(x y) cmd`) is an ordinary prefix assignment; upstream's `inline variables cannot be arrays` error stays for Bash | `syntax/parser.go`, `syntax/parser_test.go` | [#285](https://github.com/z-shell/zsh-lint/issues/285) |
| A braced `${+` must be followed by a parameter name; a nested expansion, quote, special parameter or operator after it is an error, as in Zsh | `syntax/parser.go`, `syntax/parser_test.go` | [#363](https://github.com/z-shell/zsh-lint/issues/363) |
| A glob group in any word ends the word at `;`, `&`, or a `<` or `>` that starts neither a process substitution nor a numeric glob `<m-n>`, as Zsh's lexer does; a `(` inside a parameter expansion in the group opens nothing; a `)` left over by such a word in `[[ ]]` is reported where it stands | `syntax/parser.go`, `syntax/glob_group_word_test.go`, `syntax/module_test.go` | [#511](https://github.com/z-shell/zsh-lint/issues/511) |
| Outside double quotes, braces nest in the word of a `${...}`, so `${x:-{}}` is one expansion and `${x:-{}` is unclosed; a nested expansion keeps its own count; a nested expansion, and a here-document body, count as double-quoted for the #400 quote rule | `syntax/parser.go`, `syntax/lexer.go`, `syntax/param_word_brace_test.go` | [#518](https://github.com/z-shell/zsh-lint/issues/518) |
| An `=~` operand ends inside a group at `;`, `&`, or a `<` or `>` that starts neither a process substitution nor a numeric glob `<m-n>`, as any other Zsh word does; a `)` left over after `&&` names the outermost open `(` | `syntax/lexer.go`, `syntax/parser.go`, `syntax/regex_group_word_test.go` | [#517](https://github.com/z-shell/zsh-lint/issues/517) |
| A `(` directly followed by `)` ends a word inside a glob group, a condition operand's group, or an `=~` operand, as `()` is a token of its own in Zsh; an empty process substitution `<()` is not such a `(` | `syntax/parser.go`, `syntax/lexer.go`, `syntax/empty_group_test.go`, `syntax/parser_test.go` | [#522](https://github.com/z-shell/zsh-lint/issues/522) |

## Taking an upstream release

Copy the new release's packages over this directory, reapply the changes above, and run upstream's tests here and zsh-lint's full suite, with the tree-shape checks ADR-0023 point 3 requires.
