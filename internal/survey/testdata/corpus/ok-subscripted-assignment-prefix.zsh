#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #285.
# A simple command is a sequence of optional parameter assignments followed by
# words, and a subscripted or array assignment is an assignment like any
# other. Bash forbids one before a command word; Zsh does not.
#
# Every line runs: the prints show what each prefix actually did.

emulate -L zsh

typeset -a a
a[2]=x true
print -r -- "subscript sets the element: ${(qq)a}"

typeset -A h
h[k]=v true
print -r -- "associative key: ${h[k]}"

a[3]+=y b=1 true
print -r -- "append to an element: ${(qq)a}"

a[1]=first >/dev/null print -r -- hidden
print -r -- "with a redirection: ${a[1]}"

# An array value before a command word is accepted too. The command runs, and
# the assignment does not persist.
unset c
c=(1 2) true
print -r -- "array prefix leaves c unset: ${+c}"

print done-285
