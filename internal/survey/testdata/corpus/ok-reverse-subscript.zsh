# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #15: reverse-subscript flags with a native-Zsh glob pattern.
print -r -- ${_comps[(I)-value-*]}
