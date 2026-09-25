#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Fixture for #441. A quoted `)` inside a pattern group ends the group early in
# the parser's lexer; the nested-pattern adapter masks it, but inside a
# backquoted command the parser reports the unclosed quote at the closing
# backquote rather than at the end of input, and the adapter did not
# recognise that error.
#
# Every line passes `zsh -f -n` and runs under `zsh -f`, printing its label.

x=')'
print -r -- `[[ ')' == (a|(b|")")) ]] && print -r -- double-quoted-closer`
print -r -- `[[ ')' == (a|(b|')')) ]] && print -r -- single-quoted-closer`
print -r -- `[[ ')' == (a|"${x:-)}") ]] && print -r -- closer-in-expansion-word`
print -r -- `[[ ')' == (a|(b|"${x:-)}")) ]] && print -r -- nested-group`
