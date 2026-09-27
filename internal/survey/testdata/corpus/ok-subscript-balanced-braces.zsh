# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts
# Issue #521: a balanced `{...}` in the subscript of an unquoted ${...} is
# part of the key, blanks and commas included.
typeset -A h
h=('{}' empty '{a}' one '{a b}' spaced 'x{b}y' inner '{a,b}' comma)
print -r -- ${h[{}]} ${h[{a}]} ${h[{a b}]} ${h[x{b}y]}
print -r -- ${h[(i){a,b}]} ${h[(r)one]}
print -r -- ${h[{a}]:-none} ${#h[{a}]}
