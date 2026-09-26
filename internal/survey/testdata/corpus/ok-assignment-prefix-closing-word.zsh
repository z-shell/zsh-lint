# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Issue #278: Zsh keeps command position after an assignment prefix, so a
# closing reserved word right after it ends the prefix-only command and the
# enclosing construct accepts it. Words that only look reserved stay
# arguments.

if x=1 then :; fi
if true; then x=1 fi
if true; then :; else x=1 fi
for a in b; do x=1 done
foreach a (b) x=1 end
case a in a) x=1 esac
x=1 print '!' '[[' time
x=1 \time true
x=1 true !
