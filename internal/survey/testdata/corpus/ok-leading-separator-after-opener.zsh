#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Regression fixture for #569. A `;` directly after the `(` of a subshell or
# the `{` of a brace group ends an empty sublist in Zsh, and the list runs.
# The parser fork reads it as it reads any other empty sublist.
( ; print a )
{ ; print b }
