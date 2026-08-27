# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et

# Regression fixture for issue #163.
() {
  builtin emulate -L zsh
  typeset -r source_path=$1
  print -r -- "$source_path"
} "${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
