# -*- Mode: sh; sh-indentation: 4; indent-tabs-mode: nil; sh-basic-offset: 4; -*-
# vim:ft=zsh:sw=4:sts=4:et

# Copyright (c) 2019 Sebastian Gniazdowski
# License MIT

# Handle $0 according to the Zsh Plugin Standard:
# http://z-shell.github.io/Zsh-100-Commits-Club/Zsh-Plugin-Standard.html
0="${${ZERO:-${0:#$ZSH_ARGZERO}}:-${(%):-%N}}"
0="${${(M)0:#/*}:-$PWD/$0}"

typeset -g ZSHLINT_REPO_DIR="${0:h}"
typeset -g ZSHLINT_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/zsh-lint"

autoload zsh-lint
