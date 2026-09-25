#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Fixture for #401. Inside a double-quoted `${...}` the word opens its own
# quoting context, so `"${x:-"it's"}"` holds a nested string with a literal
# quote. The nested-pattern adapter used to read the inner `"` as the end of
# the outer string and the `'` as the start of a single-quoted region that
# swallowed the pattern group after it.
#
# Every test below passes `zsh -f -n` and the file runs to completion under
# `zsh -f`, printing each label once.

[[ "${x:-"it's"}" == (a|(b|c)) ]] || print -r -- nested-string-with-quote
[[ "${x:-"its"}" == (a|(b|c)) ]] || print -r -- nested-string-without-quote
[[ "${x:-")"}" == (a|(b|c)) ]] || print -r -- quoted-closer-in-word
[[ a == (a|(b|"${x:-)}")) ]] && print -r -- closer-in-quoted-word-in-group
[[ a == (a|(b|"${x:-"it's"}")) ]] && print -r -- nested-string-in-group
