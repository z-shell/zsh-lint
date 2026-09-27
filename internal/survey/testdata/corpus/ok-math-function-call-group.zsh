#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
# Fixture for #356.
# A math function call is an operand, so it may stand inside a parenthesized
# group. The parser reports such a group as unmatched, an error the call
# adapter did not own, so the retry never ran.
#
# zsh/mathfunc supplies the named functions; every line is exercised so the
# fixture proves evaluation rather than only parsing.

emulate -L zsh
zmodload zsh/mathfunc

print "bare $(( (int(sqrt(16))) ))"

print "nested $(( ((int(sqrt(16)))) ))"

print "right-operand $(( 1 + (int(sqrt(16))) ))"

print "ternary-branch $(( 1 ? (int(sqrt(16))) : 3 ))"

# The group must keep its own shape: (4 + 1) * 2 is 10, and a mask that let
# the call absorb the neighbours would give a different value.
print "group-then-product $(( (int(sqrt(16)) + 1) * 2 ))"

print "two-calls $(( 2 * (int(sqrt(16)) - int(sqrt(4))) ))"

print "two-arguments $(( (int(fmod(7, 4))) ))"

print "no-arguments $(( (rand48()) < 2 ))"

# The arithmetic command form.
integer answer
(( answer = (int(sqrt(16))) ))
print "command $answer"

# A group with no call still parses as it always did.
print "plain $(( (1 + 2) ))"

print done-356
