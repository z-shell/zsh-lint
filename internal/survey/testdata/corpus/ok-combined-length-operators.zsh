# vim: ft=zsh
# Length is applied after suffix operators, including modifiers and slices (#612).
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
value=/one/two/file.zsh
offset=2
print -r -- ${#value:t} ${#value:h} ${#value:$offset}
print -r -- ${#*:#0} ${#${value}:-}
