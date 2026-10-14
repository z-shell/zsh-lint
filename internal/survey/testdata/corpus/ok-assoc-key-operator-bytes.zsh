# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts
# Issue #691: an associative key holding `%`, `*`, `?`, `~`, `,` or `=` is a
# literal key, but mvdan/sh reads the subscript as arithmetic and the byte
# as an operator with a missing operand.
#
# Every line runs, so a key read as anything but its literal text shows up
# as a wrong value rather than passing silently. Under zsh 5.9.2 this prints
# ` a b c`, `1 2 3 4 5 6 7`, `8 9 10 11`, then `j1` and `30 3`.
emulate -L zsh
typeset -A z e jt

# Zsh's Functions/Zle/define-composed-chars, lines 207, 381 and 424.
z[%]+=" a"
z[,]+=" b"
z[=]+=" c"
# Oh My Zsh plugins/emoji/emoji-char-definitions.zsh, line 4086.
e[keycap_*]=1
e[*a]=2
e[a?]=3
e[?a]=4
e[a?b]=5
e[~]=6
e[a~b]=7
e[a%]=8
e[%a]=9
e[a,]=10
e[a=]=11

print -r -- "$z[%]$z[,]$z[=]"
print -r -- ${e[keycap_*]} ${e[*a]} $e[a?] ${e[?a]} ${e[a?b]} ${e[~]} ${e[a~b]}
print -r -- ${e[a%]} $e[%a] ${e[a,]} ${e[a=]}

# Zsh's Functions/Prompts/prompt_bart_setup, line 91, reads the special
# $jobtexts this way; jt stands in for it here.
set -- 1
jt[%?1]=j1
[[ -n $jt[%?$1] ]] && print -r -- $jt[%?$1]

# An ordinary array subscript keeps its arithmetic.
typeset -a arr=(10 20 30)
integer i=5
print -r -- $(( arr[i%3+1] )) $(( i ^ 6 ))
