# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Rules
# Regression (was gap #53): the Plugin Standard ZERO idiom parses under LangZsh.
0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
