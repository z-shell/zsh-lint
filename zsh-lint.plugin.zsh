# Copyright (c) 2019 Sebastian Gniazdowski
# Copyright (c) 2021 Salvydas Lukosius
#
# Handle $0 according to the Zsh Plugin Standard:
# http://z-shell.github.io/Zsh-100-Commits-Club/Zsh-Plugin-Standard.html
0="${${ZERO:-${0:#$ZSH_ARGZERO}}:-${(%):-%N}}"
0="${${(M)0:#/*}:-$PWD/$0}"

typeset -g ZSHLINT_REPO_DIR="${0:h}"
typeset -g ZSHLINT_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/zsh-lint"

# According to Zsh Plugin Standart.
# https://github.com/z-shell/zi/wiki/Zsh-Plugin-Standard#2-functions-directory
if [[ $PMSPEC != *f* ]] {
    fpath+=( "${0:h}/functions" )
}

autoload zsh-lint
