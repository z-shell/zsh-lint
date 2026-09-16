# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #237: a bracket expression or escaped bracket inside a flagged subscript
# pattern; mvdan/sh ends the raw pattern at the first ']' while Zsh ends it at
# the ']' that balances the nesting.
local -a line
print -r -- ${line[(i)[a]]}
print -r -- ${line[(I)[\']]}
print -r -- ${line[(r)[ab]*]}
print -r -- ${line[(i)x[a]y[b]]}
print -r -- ${line[(i)[[:alpha:]]*]}
print -r -- ${line[(i)[a,b]]}
print -r -- ${line[(i)[^a]]:-none}
print -r -- ${line[(i)a\]b]}
print -r -- ${line[(n:2:)[a]]}
line[(i)[a]]=x
testname="${line[(( ${line[(i)[\']]}+1 )),(( ${line[(I)[\']]}-1 ))]}"
