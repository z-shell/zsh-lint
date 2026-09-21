# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #327: a while, until or for header whose body is the empty sublist
# native Zsh reads before a closer, before a pipeline or list operator, or at
# end of file. par_while and par_for in parse.c, when neither `do` nor `{`
# follows and SHORT_LOOPS is set, read one sublist, and par_sublist accepts an
# empty one. The select analogue is #302 and #319; the arithmetic for form is
# #241.
#
# An empty body needs a closer after the header: a bare header followed by
# another command on a later line takes that command as its body instead, so
# every row below is closed by `}`, `)`, a reserved word, an operator, or the
# end of the file.
#
# Corpus site: none (found by probing while verifying #211).
f() { while true }
g() { until true }
h() { while (( 1 )) }
i() { until (( 1 )) }
j() { while [[ -n $x ]] }
k() { while true print x }
l() { while (( 1 )) print x }
m() { while true; }
n() { until true; }
( while true )
{ while true }
{ until true; }
if true; then while true; fi
if true; then until true; fi
v() { while true || print x }
( while true || print x )
w() { while true | cat }
o() { for i in a b; }
q() { for i; }
r() { for i (a b) }
{ for i in a b; }
if true; then for i in a b; fi
t() { while true }; print after
u() { select o in a; }
while true
