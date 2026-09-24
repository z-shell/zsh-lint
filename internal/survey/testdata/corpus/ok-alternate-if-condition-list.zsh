#!/usr/bin/env zsh
# Fixture for #376. In the alternate forms `if list { list }` and
# `while list { list }` the condition is a list, so a plain command may head an
# `&&`/`||` or pipeline sublist and the brace after the last element is still
# the body. Minimized from z-shell/zi lib/zsh/install.zsh:396.
#
# Every form below passes `zsh -f -n`, and the file runs to completion under
# `zsh -f`: each body runs at most once.
f() { return 0 }
x=
if f arg && [[ -z $x ]] {
  print if-body
}
if true && (( 1 )) { print arith-body }
if true || [[ -z $x ]] { print or-body }
if f arg && [[ -n $x ]] { print not-run } else { print else-body }
if false && [[ -z $x ]] { print not-run } elif f && [[ -z $x ]] { print elif-body }
if print cond | read -r x && [[ -n $x ]] { print pipe-body }
if f && { true } { print group-body }
n=0
while (( n < 1 )) && true && (( n == 0 )) { (( n++ )) }
while true && [[ -z $x$n ]] { print not-run }
