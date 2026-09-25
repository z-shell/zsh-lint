#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #275. What follows the closing `}` of a brace-form `for`,
# `while` or `if` on the same line belongs to the loop or the `if`, where
# native Zsh reads it. After the `if` closer only a separator may follow;
# after `while` and `for` an operator or a redirect may too.
#
# Every line passes `zsh -f -n` and runs under `zsh -f`; the expected output
# is noted on each line.

for x (a b) { print -n $x }; print           # ab
for x (a b) { print -n $x } && print         # ab
for x (a) { false } || print or              # or
for x (a b) { print $x } | wc -l             # 2
while (( 0 )) { : }; print while             # while
while (( 0 )) { : } || print never           # (nothing)
if (( 1 )) { print -n if }; print            # if
if (( 0 )) { : } else { print -n else }; print  # else
