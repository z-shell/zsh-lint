# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #235: an associative key may mix a parameter expansion with ':'.
# zshparam "Array Subscripts" treats the subscript of an associative array as
# key text after expansion, but mvdan/sh reads the ':' after `$M` or `${M}`
# as a ternary operator missing its `?`.
typeset -A map ZI_SNIPPETS ICE
local M=a N=b MATCH=x:
x=${map[${M}:]}
x=${map[$M:b]}
x=${map[$M:$N]}
x=${map[${M}::${N}]}
x=${map[$M:b]:-default}
(( ${+map[$M:b]} )) && print -r -- ${map[$M:b]}
# F-Sy-H lib/highlight.zsh
print -r -- ${map[${MATCH%:}:]}
# zi.zsh
print -r -- ${ZI_SNIPPETS[PZT::modules/$1${ICE[svn]-/init.zsh}]}
map[$M:b]=1
map[${M}:]=1
(( ${+persisted_free[<styles>_free${style}]} )) && x=${persisted_free[<styles>_free${style}]-}
