# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #242: a bare associative key may place '-' next to '.', '@', '/', or
# the closing ']', which mvdan/sh reads as an arithmetic operator missing an
# operand.
local -A ZI A
ZI[bkp-.]="${functions[.]}"
(( ${+ZI[bkp-.]} )) && print -r -- ${ZI[bkp-.]}
A[a-@]=x
A[a-]=x
A[a.-]=x
A[a-/]=x
