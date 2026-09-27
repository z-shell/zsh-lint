#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #469.
# A dangling `&&` or `||` ends its list when the backquote that closes its
# command substitution follows, just as a closing `)` does for `$(...)`.
#
# `zsh -f -n` does not parse backquote bodies, so this fixture is also run:
# every substitution below is evaluated and its output printed.

emulate -L zsh

x=`print and-before-closer &&`
print $x

x=`print or-before-closer ||`
print $x

x=`print blank-before-closer || `
print $x

x=`print newline-before-closer ||
`
print $x

x=`print comment-before-closer || # a comment
`
print $x

x="`print in-double-quotes ||`"
print $x

x=`print first; print after-a-statement ||`
print $x

x=$(print `print inside-dollar-paren ||`)
print $x

x=`print \`print nested-inner ||\``
print $x

x=`print \`print nested-outer\` ||`
print $x

x=`print then-outer-operator ||` || print not-reached
print $x

print `print as-an-argument ||`

print done-469
