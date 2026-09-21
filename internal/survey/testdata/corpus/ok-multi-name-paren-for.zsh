#!/usr/bin/env zsh
# Fixture for #324.
# zshmisc composes two productions: a `for` header may name more than one
# variable, and the alternate form spells the word list as `( word ... )`.
# The front end handled each alone but not together.
#
# The names bind in tuples: `for a b (1 2 3 4)` runs with a=1 b=2, then a=3 b=4.

emulate -L zsh

for a b (1 2) { print "pair $a $b" }

for a b (1 2 3 4) { print "tuple $a $b" }

for a b c (1 2 3) { print "three $a $b $c" }

for a b (1 2) print "sublist $a $b"

# A nested loop inside the body.
for a b (1 2) { for c d (3 4) { print "nested $a$c" } }

# The word list keeps its own syntax: a quoted `)` does not close it.
for a b ('literal)' plain) { print "quoted $a $b" }

# The single-name short form is unchanged.
for a (1 2) { print "single $a" }

print done-324
