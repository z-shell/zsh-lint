# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Regression (was gap #11): parameter-expansion flags parse under LangZsh.
typeset -A opts
opts=( verbose 1 )
print -r -- ${(kv)opts}
