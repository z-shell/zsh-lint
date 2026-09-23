# Fixture for #250.
# A flagged subscript pattern holding a bracket expression, in the two places
# a flagged pattern may stand: at the start of a subscript, and after a range
# comma. The second-subscript adapter masks its `][` boundary as `, `, so the
# second subscript's flagged pattern reaches the pattern scanner through the
# range-comma position rather than its own `[`.
#
# Every line prints, so a mask that parsed but changed what the subscript
# selects shows up as a wrong value rather than passing silently.

emulate -L zsh
setopt extended_glob

typeset -a words
words=(alpha beta gamma delta)

typeset -A groups
groups=(first 'alpha beta' second 'gamma delta')

# A range endpoint may carry subscript flags, and its pattern may hold a
# bracket expression. Both rows select through `gamma`.
print -r -- "${words[1,(i)gamma]}"
print -r -- "${words[1,(i)[g]amma]}"
print -r -- "${words[1,(r)[a-z]#a]}"

# A second subscript whose flagged pattern holds a bracket expression. The
# first subscript picks the value, the second indexes into it.
print -r -- "${${groups[first]}[(i)[b]eta]}"
print -r -- "${groups[second][(i)[g]amma]}"

# The same, with the pattern's bracket expression written as a character
# class and quantified.
print -r -- "${groups[first][(i)[[:alpha:]]#]}"

# A nested expansion inside a flagged pattern that also holds a bracket
# expression: the expansion is the parser's own node, and the brackets
# around it belong to the pattern.
typeset needle=et
print -r -- "${groups[first][(i)[^[:space:]]#${needle}[^[:space:]]#]}"

# An escaped bracket is literal and does not open a bracket expression.
typeset -A literal
literal=(']x' found)
print -r -- "${literal[(i)\]x]}"

# The shape this file exists for, as written in z-shell/zi
# lib/zsh/autoload.zsh:1167: a flagged second subscript whose pattern holds a
# backreference group, a bracket expression and a nested expansion.
typeset -A functions_like
functions_like=(fn 'run helper once')
typeset orig=helper
integer idx="${functions_like[fn][(i)(#b)([^[:space:]]#${orig}[^[:space:]]#)]}"
print -r -- "$idx"
