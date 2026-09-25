#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #281 and the `repeat` rows of #280. The parser fork reads
# `repeat count` as its own loop, with the bodies `for` takes, wherever a
# command may appear: inside a double-quoted command substitution, in an
# unquoted here-document and in an arithmetic expansion.
#
# Every command passes `zsh -f -n` and runs under `zsh -f`; the expected
# output is noted on each line.

repeat 2 print -n a; print                             # aa
repeat 2; do print -n b; done; print                   # bb
repeat 2 { print -n c }; print                         # cc
repeat 2 repeat 2 print -n d; print                    # dddd
print "$(repeat 2 do print -n e; done)"                # ee
print $(( $(repeat 2 print -n 1) + 1 ))                # 12
# The here-document prints ff.
cat <<EOF
$(repeat 2 print -n f)
EOF
