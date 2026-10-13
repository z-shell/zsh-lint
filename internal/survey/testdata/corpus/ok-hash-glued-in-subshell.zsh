# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Comments
# Regression fixture for #660. A comment is a word that begins with `#`, so a
# `#` glued to an expansion, quote or glob group continues that word inside a
# subshell, command substitution, case arm or array element too (also #587).
x=1
( print -r -- ${x}#y )
( print -r -- 'a'#y "b"#y $x#y )
y=$( print -r -- ${x}#y )
f() ( print -r -- ${x}#y )
a=( ${x}#y 'a'#y )
case a in (a) print -r -- ${x}#y ;; esac
