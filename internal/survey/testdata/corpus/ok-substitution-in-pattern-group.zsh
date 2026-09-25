#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution
# Fixture for #439. Inside a `[[ ... == (...) ]]` pattern group, the `)` that
# closes a command or arithmetic substitution, and a `)` in a quoted word, do
# not close the group. The upstream parser ended the group at the first `)` it
# met.
#
# Every line passes `zsh -f -n` and runs under `zsh -f`; the expected output
# is noted on each line.

[[ a == (a|(b|"$(print ")")")) ]] && print hit          # hit
[[ ')' == (a|(b|"$(print ")")")) ]] && print hit        # hit
[[ ')' == (a|(b|"${x:-$(print ")")}")) ]] && print hit  # hit
[[ ')' == (a|(b|"`print ")"`")) ]] && print hit         # hit
[[ b == (a|$(print b)) ]] && print hit                  # hit
[[ b == (a|"$(print b)") ]] && print hit                # hit
[[ ')' == (a|")") ]] && print hit                       # hit
[[ ')' == (a|')') ]] && print hit                       # hit
[[ x == (a|$(print $(print x))) ]] && print hit         # hit
[[ x == (a|(b|$(print x))) ]] && print hit                # hit
[[ x == (a|(b|`print x`)) ]] && print hit                 # hit
[[ 3 == (a|(b|$((1+2)))) ]] && print hit                  # hit
[[ 3 == (a|(b|"$((1+2))")) ]] && print hit                # hit
