#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #332. A `;` in command position terminates an empty sublist,
# which Zsh reads as a no-op. Verified by running these rather than by
# reading the grammar: `print a; ;` prints `a` once.
#
# A bare loop keeps its `}` or end of file close by on purpose. A bare loop
# followed by another statement is #330, where the next statement becomes
# the loop's body, and is a separate gap from the redundant separator.
;
print top; ;
; ; ;
print run; ; print again
f() { while true; ; }
g() { until true; ; }
h() { while (( 1 )); ; }
k() { print body; ; }
m() { : ; ; }
case x in y) :;; esac
print 'quoted;' ; ;
print "also;" ; ;
n() { print a; ; print b; ; }
print done_marker; ;
