#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
# Fixture for #233.
# zshmisc, Arithmetic Evaluation: an arithmetic expression may call a math
# function, `func(args)`. The front end read the `(` after the name as an
# invalid operator.
#
# zsh/mathfunc supplies the named functions; the shapes below are what the
# parser must accept, and they are exercised so the fixture proves evaluation
# rather than only parsing.

emulate -L zsh
zmodload zsh/mathfunc

print "one-arg $(( int(sqrt(16)) ))"

print "expression-arg $(( int(sqrt(8 + 8)) ))"

print "two-arg $(( int(fmod(7, 4)) ))"

# A call as one operand of a larger expression: the call must bind where it is
# written, not capture its neighbours.
print "precedence $(( int(sqrt(16)) + 1 ))"

print "precedence-left $(( 1 + int(sqrt(16)) ))"

print "precedence-mul $(( int(sqrt(16)) * 2 + 3 ))"

# Nested calls.
print "nested $(( int(sqrt(abs(-16))) ))"

# Two calls in one expression.
print "two-calls $(( int(sqrt(16)) + int(sqrt(4)) ))"

# The arithmetic command form, and a call in a condition.
(( answer = int(sqrt(16)) ))
print "command $answer"

if (( int(sqrt(16)) > 3 )); then
  print "condition ok"
fi

# A grouping parenthesis is not a call and must keep its meaning.
print "grouping $(( (1 + 2) * 3 ))"

print done-233
