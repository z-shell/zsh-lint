# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #384: a single-quoted key inside a nested expansion, where the
# enclosing subscript carries a pattern flag. The nested-expansion scan of
# #371 refused any quote outright, because a byte count and a quote are not
# composable: a quoted bracket still moves the count. Native Zsh reads a
# single-quoted region inside a subscript literally, so one holding no
# delimiter byte is decidable and is stepped over; the double-quoted twin
# '${m[(i)${Z["a b"]}]}' is 'bad substitution' natively and stays refused.
local -A Z=( a 1 ab 2 'a b' 3 'a,b' 4 'a#b' 5 k 6 )
local -A Y=( ab k )
local -a m=( a1 abx abc 1 2 3 k )
# The reported shape, on both flags.
print -r -- ${m[(i)${Z['ab']}]}
print -r -- ${m[(r)${Z['ab']}]}
# A space in the key is why the quotes are there in real code.
print -r -- ${m[(i)${Z['a b']}]}
# Bytes that are operators unquoted are key text here.
print -r -- ${m[(i)${Z['a#b']}]}
print -r -- ${m[(i)${Z['a$b']}]}
print -r -- ${m[(i)${Z['a*b']}]}
# An empty key, and a quoted run beside unquoted text.
print -r -- ${m[(i)${Z['']}]}
print -r -- ${m[(i)${Z[a'b'c]}]}
# The positions a flagged pattern stands in: a range endpoint, a second
# subscript, and a nested outer expansion.
print -r -- ${m[1,(i)${Z['ab']}]}
print -r -- ${m[1][(i)${Z['ab']}]}
print -r -- ${${m[1]}[(i)${Z['ab']}]}
# The assignment form, and the flagged pattern inside double quotes.
m[(i)${Z['ab']}]=v
print -r -- "${m[(i)${Z['ab']}]}"
# A quoted key one level further down.
print -r -- ${m[(i)${Z[${Y['ab']}]}]}
# Composition with the unquoted shapes of #371 and the bracket expression
# of #283 in the same pattern.
print -r -- ${m[(i)${Z['ab']}${Z[a]}]}
print -r -- ${m[(i)${Z['ab']}[^:]##]}
