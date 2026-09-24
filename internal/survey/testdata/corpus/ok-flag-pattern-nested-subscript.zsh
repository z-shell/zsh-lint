# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #382: a flagged subscript whose pattern holds a nested subscripted
# expansion. The parser ends the raw pattern at the first ']', which inside the
# nested expansion belongs to the inner subscript, so the pattern is cut and the
# rest of the subscript is left over. Whether anything reported that depended on
# what the leftover bytes happened to mean: unquoted they formed a word ending
# in '}' the close-brace guard refuses, inside a double quote they were ordinary
# text nobody looked at, so the outer quote silently selected the verdict.
#
# These rows are the valid side of that family and must keep parsing. Each is
# verified with 'zsh -f -n' and by running it under 'zsh -f'.
local -A Z=( a 1 )
local -A Y=( 'a b' 7 )
local -a m=( a1 abx abc )
local -a n=( x y )

# The nested expansion closes what it opens, so the pattern is not cut.
print -r -- "${m[(i)${Z[a]}]}"
print -r -- "${m[(i)${Z[a]}x]}"
print -r -- "${m[(i)${Z[a]}$(echo x)]}"
print -r -- "${m[(i)a_bc_$(echo x)]}"
print -r -- "${m[(r)${Z[a]},3]}"

# A single-quoted key inside the nested subscript is the valid spelling of the
# shape whose double-quoted twin is 'bad substitution'. Zsh's rule here is
# asymmetric between the quote kinds, so the guard must not treat them alike.
print -r -- "${m[(i)${Y['a b']}]}"
print -r -- "${m[(r)${Y['a b']}]}"
print -r -- "${m[(i)x${Y['a b']}y]}"
print -r -- "${m[(i)${Z[${Y['a b']}]}]}"
v=${m[(i)${Y['a b']}]}
print -r -- "$v"

# The same nested expansion in a second-level subscript and a range. The range
# takes `(r)`, not `(i)`: an index flag with a range is `invalid subscript`
# whatever the pattern is, nesting or none.
print -r -- "${n[${m[(i)${Z[a]}]}]}"
print -r -- "${m[(r)${Z[a]},2]}"

# Patterns with no nesting at all, including quoted ones, are untouched.
print -r -- "${m[(i)abc]}"
print -r -- "${m[(i)"abc"]}"
print -r -- "${m[(r)a[^:]##]}"
