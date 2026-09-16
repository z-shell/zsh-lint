# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #254: a brace-form if, elif, or while ends its [[ ]] condition only at
# a `]]` that is a whole word. A `]]` inside a bracket class, a pattern, or a
# quoted string belongs to the condition.
local a b __arg __style braces_stack __arg_type
typeset -A _fsh_assigns_seen
f() { print x }
if [[ $a == ([\]]) ]] { b=1 }
if [[ $a == [^\]] ]] { b=1 }
if [[ $a == ([]]) ]] { b=1 }
if [[ $a == [\]]] ]] { b=1 }
if [[ $a == [[:alpha:]] ]] { b=1 }
if [[ $a == x]] ]] { b=1 }
if [[ $a == "]]" ]] { b=1 }
if [[ $a == ']]' ]] { b=1 }
if [[ $a == "x ]] y" ]] { b=1 }
if [[ $a == $b[1]] ]] { b=1 }
while [[ $a == ([\]]) ]] { b=1 }
if [[ $a == x ]] { b=1 } elif [[ $a == [^\]] ]] { b=2 }
if [[ $a == x ]] && [[ $b == "]]" ]] { b=1 }
if [[ ( $a == x )]] { b=1 }
if [[ ($a == x)]] { b=1 }
if [[ $a == x \
]] { b=1 }
if [[ $a == $'x\' ]] y' ]] { b=1 }
if [[ $a == $'x ]] y' ]] { b=1 }
if [[ $a == (x)]] ]] { b=1 }
if [[ $a == [\)]] ]] { b=1 }
if [[ ($a == x)&&($b == y)]] { b=1 }
if [[ ! ($a == x)]] { b=1 }
if [[ $a == x&&$b == y ]] { b=1 }
if [[ $( f ) == x ]] { b=1 }
if [[ $(( a + 1 )) -gt 2 ]] { b=1 }
if [[ $a == (x|y z) ]] { b=1 }
# F-Sy-H lib/highlight.zsh
if [[ $__arg == [a-zA-Z_][a-zA-Z0-9_]#(|\[[^\]]#\])(|[^\]]#\])(|[+])=* || $__arg == [0-9]##(|[+])=* || ( $braces_stack = T* && ${__arg_type} != 3 ) ]] {
  __style=${_fsh_theme_name}assign
  _fsh_assigns_seen[${__arg%%=*}]=1
}
