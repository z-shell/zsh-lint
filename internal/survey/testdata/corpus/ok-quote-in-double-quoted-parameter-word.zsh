#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
# Fixture for #400. Inside double quotes a `'` is an ordinary character,
# including in the word of a `${name:-word}`, `${name#pattern}` or
# `${name/pattern/repl}` expansion. The upstream lexer read it as the start of
# a single-quoted string, which rejected an odd count and gave an even count
# the wrong extent.
#
# Every line passes `zsh -f -n` and runs under `zsh -f`; the expected output
# is noted on each line.

print -r -- "${x:-it's}"          # it's
print -r -- "${x:-'}"             # '
print -r -- "${x#'}"              # (empty)
y="${z:-'}"; print -r -- "$y"     # '
print -r -- "${x:-'a}'b}"         # 'a'b}
print -r -- "${x:-'}" "${w:-'}"   # ' '
print -r -- "${x/'/y}"            # (empty)
print -r -- "${x:-${w:-'}}"       # '
print -r -- ${x:-'a b'}           # a b
