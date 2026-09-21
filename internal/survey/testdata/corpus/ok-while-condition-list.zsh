#!/usr/bin/env zsh
# Fixture for #330. A `while` or `until` condition is a *list*, so it absorbs
# every sublist up to the end of the enclosing list and the body stays empty.
#
# Established by executing the forms, not by reading the grammar. If the
# following line were the body, a false condition would run it zero times:
#
#   while (( 0 ))    prints forever  => the sublist is in the CONDITION
#   print X
#
#   while (( 1 ))    terminates      => the last element is the condition
#   false
#
# This file terminates under `zsh -f`, verified by running it, so it is not a
# trap for anyone who checks it by executing rather than parsing.
#
# That constraint shapes the layout. A bare loop's condition swallows every
# later sublist in its enclosing list, so two bare loops in one list can never
# both halt: the first re-runs the second forever. Each one below is therefore
# bounded by a closer -- a function body, a subshell, or a `do ... done` -- and
# the single top-level bare loop comes last. The unbounded shapes are asserted
# in while_condition_list_test.go, where only the parse and tree shape matter.
h() { while (( 1 )); false }
k() {
while (( 1 ))
false
}
u() {
until false
true
}
( while true
false
)
a() { while true && false; false }
for i in 1 2; do while (( 1 )); false; done
while true
false
