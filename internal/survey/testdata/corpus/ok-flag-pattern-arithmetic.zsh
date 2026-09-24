# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #368: a flagged subscript pattern holding a bracket expression, in
# arithmetic. mvdan/sh ends the pattern at the bracket expression's own ']'
# and reads the rest as arithmetic, so the byte after the cut is reported as
# an operator ('#'), a missing operand ('*'), or a stray ')' ('.').
# Each line prints values, so running the file checks the evaluation too.
# extended_glob makes '#' and '##' repetition operators, so the (i) rows
# find a match rather than reporting one past the end.
setopt extended_glob
local -a m files
local -i j i
m=( a:1 abc a:2 bx a. ab. )
files=( a1.zsh b.zsh x1 )
print -r -- $(( m[(r)a[^:]##] )) ${$(( m[(r)a[^:]##] ))} $(( m[(i)a[^:]##] ))
print -r -- $(( m[(i)a[bc]] )) $(( m[(i)a[bc]] + 1 )) $(( m[(i)a[bc]#] ))
print -r -- $(( m[(i)[ab]*] )) $(( m[(i)a[bc].] ))
print -r -- $(( m[(i)[abc]##] * 2 ))
(( m[(i)a[^:]##] == 2 )) && print -r -- 5
print -r -- $(( files[(i)x[0-9]] )) $(( m[(I)[ab]*] - 2 ))
# Two cut patterns in one expression, and a radix constant beside one.
print -r -- $(( m[(i)a[bc]] + m[(i)b[xy]] + 2#11 ))
# Balanced parentheses in the pattern: Zsh counts them to find the '))'.
print -r -- $(( m[(i)(a|b)[bc]] )) $(( m[(i)a[b()c]] ))
# The same cut outside arithmetic: after a length prefix, and before a ':'.
print -r -- ${#m[(i)a[bc]]} ${m[(i)a[bc]:]}
for (( i = files[(i)x[0-9]]; i < 4; i++ )) j=m[(i)a[bc]]
print -r -- $j
