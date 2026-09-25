# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #321: `!` may precede any pipeline, so a negated select in its
# sublist, brace, parenthesized-list and empty-body forms, and a negated
# for in its brace form, are valid. Every row sits in a function body:
# the repository's `zsh -n` gate reads the exit status, and `-n` still
# applies a top-level `!` to the not-executed command's status (#287).
# Corpus site: none (found while probing #319). The for row opens its own
# line: a for after `{` on the same line is a separate gap (#274).
f1() { ! select o in a b; break }
f2() { ! select o in a b; { print $o; break } }
f3() { ! select o in a b; }
f4() { ! select o (a b) break }
f5() { ! select o (a b) { break } }
f6() { ! select o in a b; | cat }
f7() { ! select o in a b; do break; done }
f8() {
  ! for x (a b) { print $x }
}
f9() {
  ! select o in a b; break
  !	select p in c d; break
  true && ! select r in g h; break
  if ! select s in i j; break; then :; fi
  select t in k l; break; ! select u in m n; break
  ! select v in o p; ! { break }
}
