#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #394. `until list { list }` is the alternate form listed next to
# `while list { list }` in zshmisc, Alternate Forms For Complex Commands; the
# brace after the condition list is the loop body.
#
# Every form below passes `zsh -f -n`, and the file runs to completion under
# `zsh -f`: each condition is true by the time it is first tested or after
# one pass of the body.
i=0
until (( i >= 2 )) { print -r -- $i; (( i++ )) }
x=
until [[ -n $x ]] { print -r -- once; x=1 }
until [[ -n $x ]] && [[ -n $i ]] { print -r -- not-run }
until true && [[ -n $x ]] { print -r -- not-run }
until (( 1 )) { }
until (( 1 )) {
  print -r -- not-run
}
f() {
  until (( 1 )) { print -r -- not-run }
}
f
if true; then
  until [[ -n $x ]] { print -r -- not-run }
fi
print -r -- end
