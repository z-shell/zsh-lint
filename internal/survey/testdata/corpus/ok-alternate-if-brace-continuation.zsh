# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
# Issue #207: a line continuation between `}` and `else`/`elif` keeps the
# brace-form if chain intact. Minimized from z-shell/zi lib/zsh/install.zsh
# (four sites around line 1688).
if (( $#list > 1 && ${+commands[anbox]} == 1 )) { stripped=( ${(M)list[@]:#*android*} ) } \
else { stripped=( ${list[@]:#*android*} ) }
if [[ -n $x ]] { print one } \
elif (( y )) { print two } \
else { print three }
