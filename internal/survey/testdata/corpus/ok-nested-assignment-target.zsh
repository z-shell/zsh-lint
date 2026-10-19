# vim: ft=zsh
# Issue #735: top-level nested assignment-target expansions retain their children.
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Element-Assignment
items=(keep remove tail)
typeset -A state=(key remove)
items[${items[(ie)${state[key]}]}]=()
print -r -- ${(j:,:)items}
