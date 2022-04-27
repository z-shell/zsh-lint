# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
#
# Copyright (c) 2021 Salvydas Lukosius
#
# Standardized $0 Handling
# https://z.digitalclouds.dev/community/zsh_plugin_standard#zero-handling
0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
0="${${(M)0:#/*}:-$PWD/$0}"

typeset -g ZSHLINT_REPO_DIR="${0:h}"
typeset -g ZSHLINT_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/zsh-lint"

# Functions Directory
# https://z.digitalclouds.dev/community/zsh_plugin_standard#funtions-directory
if [[ $PMSPEC != *f* ]] {
    fpath+=( "${0:h}/functions" )
}

autoload zsh-lint
