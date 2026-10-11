# -*- mode: sh; sh-shell: zsh; -*-
# Initiative #612: output base and character values in native arithmetic.
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
key=a
print -r -- $(( [##16] #key ))
print -r -- $(( [#16_4] 65536 ))
print -r -- $(( 1 [#16] + 2 ))
print -r -- $(( ##a + ##\n ))
