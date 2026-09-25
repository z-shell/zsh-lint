# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags
# Issue #283: a bracket expression before '#', '*' or ',' in a flagged
# subscript pattern. mvdan/sh ends the raw pattern at the first ']', and the
# byte after that premature close decides the outcome: '#' and '##' are valid
# Zsh operators, so the file parses with the wrong tree and no error at all;
# ',' is bash's case-modification operator, a language error; '*' is no
# operator, the error #237 handled, but its scanner stopped at the ',' after.
local -A m
local -a n
local x
# The silent rows: these parse on main, with the pattern cut at the bracket
# expression's ']' and the rest of the subscript read as an operator's word.
print -r -- ${m[(r)a[^:]##]}
print -r -- ${m[(r)[^:]##,a]}
print -r -- ${m[(r)a[^:]#,b]}
print -r -- ${m[(r)a[^:]##]:-none}
print -r -- ${m[(r)${x}[^:]##]}
print -r -- ${m[a][(r)b[x]##]}
print -r -- ${m[(r)a[^:]##]} ${n[(r)b[x]#,z]}
m[(r)a[^:]##]=1
# The language-error rows: a ',' after the cut.
print -r -- ${m[(r)[^:],a]}
print -r -- ${m[(i)[a],3]}
print -r -- ${m[1,(i)[x]##]}
m[(r)[^:],a]=1
m[(r)[^:],a]+=1
# The operator-error row: a '*' after the cut, then a range ','.
print -r -- ${m[(r)[^:]*,a]}
m[(r)[^:]*,a]=1
# Composition with #277: a bracket expression on both sides of the range ','.
print -r -- ${m[(r)[^:],--[x]##]}
print -r -- ${m[(r)a[^:]##,--[x]]}
# An anonymous function invocation word holds the same shape; its words are
# typed metadata rather than tree nodes, so they are repaired separately.
() { print -r -- $1 } ${m[(r)a[^:]##]}
