# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Issue #518: outside double quotes Zsh nests braces in the word of a
# parameter expansion, so `${u:-{}}` is one expansion whose default is `{}`.
# Inside double quotes the first `}` closes it. Every line prints its value.
local u y=q
print -r -- ${u:-{}}
print -r -- ${u:-a{b}c}
print -r -- ${u:-{a}b}
print -r -- ${u:-{{}}}
print -r -- ${u:-${u:-{}}}
print -r -- ${u:-{$y}}
print -r -- ${u:-{${y}}}
print -r -- ${u:+{}}x
x=${u:-{}}
print -r -- $x
print -r -- "${u:-{}"
[[ ${u:-{}} == '{}' ]] && print match
