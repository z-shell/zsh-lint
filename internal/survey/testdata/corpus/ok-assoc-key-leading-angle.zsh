# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #236: a bare associative key may begin with '<', which mvdan/sh reads
# as an arithmetic operator without a left operand.
local -A map persisted_free
x=${map[<a>]}
map[<styles>_free]=1
print -r -- ${persisted_free[<styles>_free]-}
