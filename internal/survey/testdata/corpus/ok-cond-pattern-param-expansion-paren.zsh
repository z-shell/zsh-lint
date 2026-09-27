# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Issue #513: a `(` or `)` inside an unquoted parameter expansion in a
# conditional pattern opens or closes no group, since Zsh counts no
# parentheses inside `${...}`. The parameters are set, so each default word
# is lexed but never substituted; every line prints when it runs.
local x=b y=q
[[ ab == a(${x:-(}) ]] && print 'match 1'
[[ ab == a(${x:-(}|c) ]] && print 'match 2'
[[ b == (${x:-(}) ]] && print 'match 3'
[[ b == (a|(c|${x:-(})) ]] && print 'match 4'
[[ b == (a|(c|${x:-)})) ]] && print 'match 5'
[[ q == ((${y:-(})|c) ]] && print 'match 6'
[[ abc == *${x:-(}* ]] && print 'match 7'
[[ c != a(${x:-(}) ]] && print 'match 8'
