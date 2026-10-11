# -*- mode: sh; sh-shell: zsh; -*-
# Initiative #612: output formats following postfix arithmetic operators.
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
# Top-level synthetic expressions, with observable postfix side effects.
key=16
print -r -- $(( key++ [#16] )) $key
print -r -- $(( key--[##16] [#8] + 1 )) $key
