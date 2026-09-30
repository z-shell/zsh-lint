#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Gap fixture for #569. A `;` directly after the `(` of a subshell or the `{`
# of a brace group ends an empty sublist in Zsh, and the list runs. zsh-lint
# rejects it with the opener's own error, before the redundant-separator
# adapter can see the `;`.
( ; print a )
{ ; print b }
