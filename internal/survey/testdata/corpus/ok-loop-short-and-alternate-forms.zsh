#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #459. The parser fork reads the alternate and short forms of
# `for`, `select`, `if`, `while` and `until` as zsh's par_for, par_if and
# par_while do, including shapes the former adapters missed: a short form in
# a short body, a `{ ... }` body after an `in` list, a `for` after `{` on the
# same line (#274), a comment or `;` in the paren list (#269, #270), and a
# quoted `}` in a brace body (#271).
#
# Every line passes `zsh -f -n` and runs under `zsh -f`; the expected output
# is noted on each line.

for x (a b) print -n $x; print                         # ab
for x in a b; { print -n $x }; print                   # ab
for x y in a b c d; print -n $y; print                 # bd
for x (a;b) print -n $x; print                         # ab
for x (
  a # note
  b
) print -n $x; print                                   # ab
{ for x (a b) { print -n $x } }; print                 # ab
for x (a b) { print -n "}$x" }; print                  # }a}b
for x in a b; if [[ $x == b ]] print -n $x; print      # b
for ((i = 0; i < 2; i++)) print -n $i; print           # 01
i=0; while (( i < 2 )) (( i++ )); print $i             # 2
until (( i == 0 )) (( i-- )); print $i                 # 0
if (( 1 )) print yes                                   # yes
print "$(for x (a b) { print -n $x })"                 # ab
