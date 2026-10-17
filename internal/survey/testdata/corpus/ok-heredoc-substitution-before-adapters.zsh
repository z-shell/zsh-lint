# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
# Issue #703: a substitution drops only its own bodyless here-document.
print $(cat <<EOF)
[[ x == ((a|b)|c) ]]
{ local x=1 }
print after
