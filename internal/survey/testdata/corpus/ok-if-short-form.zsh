# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #210: the if list sublist short form of the alternate if.
# Preserved as permanent regression coverage.
local -i i
for i in {1..5}; do
  if (( i > 3 )) continue
  if [[ -n $i ]] print -r -- "$i"
done
if { true } print -r -- yes && print -r -- both
if (( 1 )) && [[ 1 ]] print -r -- chained; print -r -- after
if (( 0 )) if (( 1 )) print -r -- nested
if (( 1 )) cat <<EOF
heredoc body
EOF
