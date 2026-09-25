#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Arithmetic-Expansion
# An arithmetic expansion used as the nested parameter of a parameter
# expansion, `${$(( expr ))}` and `${(flags)$(( expr ))}` (zshexpn, Parameter
# Expansion). Native Zsh reads `$((` as arithmetic exactly when the first
# unbalanced `)` is followed by another `)`; otherwise it is a command
# substitution whose command starts with a subshell. mvdan/sh reads every
# nested `$((` as a command substitution, so a subscript in the expression
# failed as an assignment target (#361), and a comparison became a redirect.
# Every row prints, so a wrong reading shows as a wrong value.

typeset -a nums
nums=(7 8 12)
typeset -A ZI
ZI[x]=0.0061
typeset e=2 entry=x x=5

# The subscripted operand, literal and parameter index.
print -r -- "${(l:5:)$(( nums[1] ))}"
print -r -- "${(l:5:)$(( nums[$e] ))}"

# The z-shell/zi autoload.zsh shape: padding, an associative subscript and a
# pattern removal on the result.
print -r -- "${(l:5:: :)$(( ZI[$entry] * 1000 ))%%[,.]*} ms"

# Every nested-parameter prefix position.
print -r -- ${$(( nums[1]++ ))} ${nums[1]}
print -r -- "${#$(( nums[3] * 100 ))}"
print -r -- ${(l:4:)"$(( nums[2] ))"}
print -r -- ${=$(( nums[2] ))}
print -r -- ${${(l:3:)$(( nums[2] ))}}

# Two sites on one line, and one site inside another.
print -r -- ${(l:3:)$(( nums[1] ))} ${(r:3:)$(( nums[2] ))}
print -r -- ${$(( ${$(( nums[2] ))} + nums[3] ))}

# An expression across lines.
print -r -- ${$((
  nums[2] + 2
))}

# Operators that a command reading turns into a redirect or a background job.
print -r -- ${$(( x > 3 ))} ${$(( x & 1 ))} ${$(( x << 2 ))}

# Command substitutions that start with a subshell stay command substitutions.
print -r -- ${$((print sub) )}
print -r -- ${$((print one); (print two))}
