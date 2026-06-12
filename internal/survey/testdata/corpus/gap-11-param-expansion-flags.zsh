# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #11: Zsh parameter-expansion flags are not part of the Bash grammar.
typeset -A opts
opts=( verbose 1 )
print -r -- ${(kv)opts}
