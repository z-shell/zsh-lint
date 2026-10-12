# -*- mode: sh; sh-shell: zsh -*-
# Issue #625: full-string replacement, including the powerlevel10k form.
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion

typeset replacement_value=a
print -r -- ${replacement_value:/a/b}
print -r -- ${replacement_value:/a/}
print -r -- "${replacement_value:/(#b)(*)/${match[1]}}"
print -r -- ${replacement_value:/$pattern/${replacement:-b}}
