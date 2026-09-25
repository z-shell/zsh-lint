# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags
# Issue #379: a command substitution, a grave-accent substitution or an
# arithmetic expansion straight after a bracket expression in a flagged
# subscript pattern. mvdan/sh reads the whole flagged pattern as one raw
# literal and ends it at the first ']', so the bracket expression's ']' cut
# the pattern and the '$' after it was read as a parameter-expansion
# operator. The repair steps over the substitution while masking its ','
# so the literal reaches the ']' that really closes the subscript.
local -a m=( a1 abx abc )
local x=c
# The reported shapes, every flag and every substitution form.
print -r -- ${m[(i)a[bc]$(echo x)]}
print -r -- ${m[(i)a[bc]`echo x`]}
print -r -- ${m[(i)a[bc]$(( 1 ))]}
print -r -- ${m[(i)[ab]$(echo x)*]}
print -r -- ${m[(r)a[bc]$(echo c)]}
print -r -- "${m[(i)a[bc]$(echo x)]}"
# The assignment form, a different entry point in the front end. The
# subscript resolves to index 2 ('abx'), so the array reads a1,1,abc after
# it; printing the array proves the assignment landed where Zsh puts it.
m[(i)a[bc]$(echo x)]=1
print -r -- ${(j:,:)m}
m[2]=abx
# The same shape inside arithmetic, where the bytes after the cut are read
# as arithmetic rather than as an expansion operator.
print -r -- $(( m[(i)a[bc]$(echo x)] ))
print -r -- $(( m[(i)a[bc]`echo x`] ))
# A ',' inside the substitution is the substitution's own byte, not the
# range separator of the enclosing subscript: this must stay one pattern.
print -r -- ${m[(i)a[bc]$(echo a,b)]}
# A real range endpoint after the pattern still parses as a range.
print -r -- ${m[(r)a[bc]$(echo c),3]}
# Nesting inside the substitution: a parameter expansion, another command
# substitution, an arithmetic expansion and a parenthesized list.
print -r -- ${m[(i)a[bc]$(echo $x)]}
print -r -- ${m[(i)a[bc]$(echo $(echo c))]}
print -r -- ${m[(i)a[bc]$(echo $((1)))]}
print -r -- ${m[(i)a[bc]$( (echo x) )]}
# Two substitutions, and a bracket expression between them.
print -r -- ${m[(i)a[bc]$(echo a)$(echo b)]}
print -r -- ${m[(i)a[bc]$(echo a)[de]$(echo b)]}
# A substitution before the bracket expression rather than after it.
print -r -- ${m[(i)$(echo a)[bc]]}
# A length prefix and a suffix operator over the repaired subscript.
print -r -- ${#m[(i)a[bc]$(echo x)]}
print -r -- ${m[(i)a[bc]$(echo x)]:-none}
