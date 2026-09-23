# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #362: an associative key holding `--` or `++` is a literal key, but
# mvdan/sh reads the subscript as arithmetic and the doubled sign as a
# decrement or increment with nothing to apply to.
#
# Every line runs, so a key read as anything but its literal text shows up
# as a wrong value rather than passing silently. Under zsh 5.9 this prints
# `1 1 3 4`, `5 6 7 8 9`, `C;C;`, then `30 2`.
emulate -L zsh
typeset -A g owner_to_group
typeset o=x s=y c=C

g[${o}--$s]=1
g[$o++$s]=2
g[a--b]=3
g[a++b]=4
g[--$s]=5
g[${o}--]=6
g[1--x]=7
g[a--b--c]=8
typeset g[p--q]=9

print -r -- ${g[${o}--$s]} ${g[$o--$s]} ${g[a--b]} ${g[a++b]}
print -r -- "${g[--$s]} ${g[${o}--]} ${g[1--x]} ${g[a--b--c]} ${g[p--q]}"

# z-shell/zi lib/zsh/autoload.zsh:2351, the corpus site.
owner_to_group[${o}--$s]+="$c;"
owner_to_group[${o}--$s]+="$c;"
print -r -- $owner_to_group[${o}--$s]

# An ordinary array subscript keeps its arithmetic: these still decrement.
typeset -a arr=(10 20 30)
integer i=3
print -r -- ${arr[i--]} $i
