# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #208: Zsh repeat loops (repeat count sublist, do/done and brace bodies).
# Preserved as permanent regression coverage.
repeat 3 print -r -- tick
repeat 2; do
  print -r -- do-body
done
repeat 2 {
  print -r -- brace-body
}
repeat $(( 1 + 1 )) repeat 2 {
  print -r -- nested
}
integer count=0
repeat 4 (( count++ ))
print -r -- "$count"
