#!/usr/bin/env zsh
# Fixture for #354.
# A math function call is an operand, so it may stand anywhere in a ternary's
# branches. The retry is gated on the parser's first error, and a call after a
# `?` reports the unfinished ternary instead of the `(`, so the adapter never
# ran.
#
# zsh/mathfunc supplies the named functions; every line is exercised so the
# fixture proves evaluation rather than only parsing.

emulate -L zsh
zmodload zsh/mathfunc

print "true-branch $(( 1 ? int(sqrt(16)) : 3 ))"

print "false-branch $(( 0 ? 3 : int(sqrt(16)) ))"

print "both-branches $(( 1 ? int(sqrt(16)) : int(sqrt(4)) ))"

print "condition-and-true $(( int(sqrt(16)) ? int(sqrt(16)) : 3 ))"

print "two-arg-true $(( 1 ? int(fmod(7, 4)) : 3 ))"

# The true branch as part of a larger expression: the ternary must keep its
# own shape rather than absorb the neighbours.
print "true-branch-expression $(( 1 ? int(sqrt(16)) + 1 : 3 ))"

# A nested ternary, with the call in the inner true branch.
print "nested $(( 1 ? (1 ? int(sqrt(16)) : 3) : 5 ))"

# A call is an operand, so it stands anywhere an operand may, not only
# immediately after the `?`. Each of these reports the same ternary error.
print "unary-true $(( 1 ? -int(sqrt(16)) : 3 ))"

print "product-true $(( 1 ? 2 * int(sqrt(16)) : 3 ))"

print "sum-true $(( 1 ? 2 + int(sqrt(16)) : 3 ))"

# Arithmetic is not line-oriented, so a ternary may be written across lines.
# This is the shape the gap was found in: z-shell/zi zi.zsh:2749 breaks
# immediately after the `?`.
print "multiline $(( 1 ?
  int(sqrt(16)) : 3 ))"

# The arithmetic command form.
integer answer
(( answer = 1 ? int(sqrt(16)) : 3 ))
print "command $answer"

# A ternary with no call at all still parses as it always did.
print "plain $(( 1 ? 2 : 3 ))"

print done-354
