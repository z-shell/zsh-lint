# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags
# Issue #371: a flagged subscript pattern holding a nested parameter
# expansion that itself carries a subscript. mvdan/sh reads the whole
# flagged pattern as one raw literal and ends it at the first ']', so the
# nested expansion's own ']' cut the pattern and the outer '}' then closed
# nothing. A nested ',' split the index into a range the source does not
# have.
local -A Z=( a q k j )
local -a m=( p q r )
local -a manpath=( /usr/share/man )
local k=a
# The reported shapes: a nested subscripted expansion inside the pattern.
print -r -- ${m[(r)${Z[a]}]}
print -r -- ${m[(re)${Z[a]}]}
print -r -- ${m[(i)${Z[a]}]}
print -r -- ${m[(r)${Z[a]}x]}
print -r -- ${m[(r)${(q)Z[a]}]}
print -r -- ${m[(r)${Z[${k}]}]}
# The zi.zsh idiom the gap blocked, at lines 189, 199, 205, 220 and 226.
[[ -z ${manpath[(re)${Z[a]}]} ]] && print -r -- absent
# A nested ',' must not become a range separator.
print -r -- ${m[(r)${m[1,2]}]}
# The assignment form, which parsed before this fix, and a quoted one.
x=${m[(r)${Z[a]}]}
print -r -- "${m[(r)${Z[a]}]}"
# Composition with the bracket expression of #283 in the same pattern.
print -r -- ${m[(r)${Z[a]}[^:]##]}
# A range endpoint's flagged pattern, and a second subscript's.
print -r -- ${m[1,(i)${Z[a]}]}
print -r -- ${Z[a][(i)${Z[k]}]}
# An escaped bracket inside the nested subscript: the escape means it is
# not a subscript delimiter, so it is masked like the outer scan masks its
# own and does not move the balance count.
print -r -- ${m[(r)${Z[a\]b]}]}
print -r -- ${m[(r)${Z[a\[b]}]}
