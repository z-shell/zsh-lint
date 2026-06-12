# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #53: nested parameter expansions (Z-Shell Plugin Standard ZERO idiom).
0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
