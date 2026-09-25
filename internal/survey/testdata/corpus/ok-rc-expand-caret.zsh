# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Manual: https://zsh.sourceforge.io/Doc/Release/Options.html#index-RC_005fEXPAND_005fPARAM
# Issue #60: rc-expand caret syntax from zshexpn and RC_EXPAND_PARAM.
print -- ${^manpath}
# Issue #196: rc-expand caret after a parameter flag group.
for sequence in {a,i}${(s..)^:-'()[]{}<>bB'}; do print -r -- $sequence; done
