# vim: set ft=zsh:
# #266: arithmetic command inside a command substitution, in an assignment word.
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution
output="$((( 1 )) && print -r -- ok)"
