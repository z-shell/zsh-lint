#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Fixture for #693. Zsh lexes the right operand of a binary condition with
# parentheses as part of the word (par_cond_2 in Src/parse.c), so a numeric or
# file comparison takes a nested group as one operand, which -eq then
# evaluates as arithmetic. The first two lines are the shapes in the Zsh
# completion functions _ipset and _adb.
#
# Every line passes `zsh -f -n` and runs under `zsh -f`; the expected output
# is noted on each line.

offset=1 argcount=2
[[ 5 -eq ((offset+4)) ]] && print hit             # hit
[[ 4 -gt (( argcount + 2 )) ]] || print miss      # miss
[[ 6 -eq (((offset+4)+1)) ]] && print hit         # hit
[[ 5 -ne ((offset)) && 1 -eq 1 ]] && print hit    # hit
[[ ! 5 -lt ((offset+4)) ]] && print hit           # hit
[[ ( 5 -le ((offset+4)) ) ]] && print hit         # hit
[[ 5 -ge (offset+4) ]] && print hit               # hit
