# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts
# Issue #372: a bare associative key containing a space is read as literal key
# text when the word after the space cannot continue an arithmetic expression.
emulate -L zsh
typeset -A Z ZI_EXTS
x=x

Z+=(
  "a b" 9
  "a b:x" 11
  " a b " 13
)
ZI_EXTS+=(
  "z-annex subcommand:x" 7
)

print -r -- ${Z[a b]}
print -r -- "${Z[a b]}"
v=${Z[a b]}
print -r -- $v
print -r -- ${Z[a b:$x]}
print -r -- "${Z[ a b ]}"
print -r -- ${(q)Z[a b]} ${#Z[a b]} ${+Z[a b]}
reply=( ${ZI_EXTS[z-annex subcommand:${(q)1-x}]} )
print -r -- "${reply[@]}"

# Arithmetic subscripts with spaces continue to evaluate as arithmetic.
typeset -a arr=(10 20 30)
integer i=1
print -r -- ${arr[i + 1]}
