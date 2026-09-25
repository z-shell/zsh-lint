# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
# Issue #232: a quoted or escaped `)` inside a conditional pattern group does
# not close the group. The parser lexes a group up to the first `)` no matter
# how it is quoted, so zsh-lint masks that byte locally and restores it.
local b=x __arg=x _mybuf=x line=x
[[ $b = (x")"y) ]] && print matched
[[ $b = (x')'y) ]] && print matched
[[ $b = (x$')'y) ]] && print matched
[[ $b = (x\)y) ]] && print matched
[[ $b = ([\)]) ]] && print matched
[[ $b = ([")"]) ]] && print matched
[[ $b = (x"\)"y) ]] && print matched
[[ $b = (x"a)b"y) ]] && print matched
[[ $b = (")") ]] && print matched
[[ $b = (a|(x")"y)) ]] && print matched
[[ $b = (x")"y)z ]] && print matched
[[ $b != (x')'y) ]] && b=1
if [[ $b == (x")"y) ]]; then b=1; fi
# F-Sy-H lib/highlight.zsh:752
[[ $__arg = (#b)*=(\()*(\))* || $__arg = (#b)*=(\()* ]] && {
  print matched
}
# F-Sy-H lib/string-highlight.zsh:20
while [[ $_mybuf = (#b)([^"{}()[]\\\"'"]#)((["({[]})\"'"])|[\\](*))(*) ]]; do
  _mybuf=${_mybuf[2,-1]}
done
# zi lib/zsh/git-process-output.zsh:133
if [[ "$line" = (#b)"Receiving objects:"[\ ]#([0-9]##)%([[:blank:]]#\(([0-9]##)/([0-9]##)\)|)* ]]; then
  print matched
fi
if [[ $b == ((a|b)|(x")"y)) ]] { b=1 } else { b=2 }
