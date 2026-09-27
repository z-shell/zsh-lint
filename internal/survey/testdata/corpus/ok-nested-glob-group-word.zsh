# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Issue #397: a bare `(` inside a glob group in a command or array word
# nests, so only its matching `)` closes it. Each pattern runs in an empty
# directory with nullglob and matches nothing, and the case pattern matches
# its subject; every line prints when it runs.
setopt extended_glob null_glob
cd "$(mktemp -d)" || exit 1
f=( (a|(b|c)) ) && print 'match 1'
f=( (a|.(b|c)) x(a|(b|c)) ) && print 'match 2'
f=( *~(a|.(b|c))/* ) && print 'match 3'
files=( (#i)**/*.(zip|rar)~(*/*|.(_backup|git))/*(-.DN) ) && print 'match 4'
f=( a(b(c)d) ) && print 'match 5'
print -r -- (a|(b)) a(b(c|d)|(e)) 'match 6'
x=a(b(c)) && print -r -- $x 'match 7'
case abc in a(b(c)|d)) print 'match 8' ;; esac
