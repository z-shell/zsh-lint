# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Regression fixture for #659. After `${#`, a second `#` names the parameter
# `#`, so `${##x}` removes a leading `x` from `$#`; only `${##}` is the length
# of `$#`.
print -r -- ${##x} ${###1} "${##*}" ${##}
