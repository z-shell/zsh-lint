# vim: ft=zsh
# Issue #612: arithmetic names are expanded before assignment and increment.
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
# Test fixture, top-level native Zsh arithmetic commands.
typeset -a values=(0)
typeset target=values suffix=counter
typeset -i a_counter=0
(( ${target}[1] = 2 ))
(( ++a_$suffix ))
