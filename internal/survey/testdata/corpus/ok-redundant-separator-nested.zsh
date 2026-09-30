#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #569. A command substitution inside double quotes and a
# backquoted command each run a list of their own, so a `;` that opens the
# list ends an empty sublist and does nothing, as it does unquoted (#332).
# Verified by running it: every line prints its words and nothing else.
print "$( ; print a )"
x="$( ; print b )"; print $x
f() { print "$( ; print c )"; }; f
print "d $( ; print e ) f"
print "$(print g; ; print h)"
print "$(print "$( ; print i )")"
print `; print j`
print "`; print k`"
print "${y:-l} $( ; print m )"
