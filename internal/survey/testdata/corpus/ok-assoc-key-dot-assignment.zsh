# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Element-Assignment
# Issue #245: an assignment whose bare associative key begins with '.' fails in
# the assignment form only; mvdan/sh reports the error one byte after the name
# start instead of at '['.
local -A ZI
functions[.]=':zi-tmp-subst-source "$@";'
(( ${+ZI[bkp-.]} )) && functions[.]="${ZI[bkp-.]}" || unfunction . 2> /dev/null
ZI[.foo]=x
