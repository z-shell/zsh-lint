# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #319: a select header with an empty body may be the left operand of
# a pipeline or list operator: native par_sublist reads the empty sublist
# and the loop is then piped or chained like any complex command. Both the
# `in` form (#302) and the parenthesized form (#303) reach the same path.
# A negated row (`! select o in a; | cat`) lives in the parser tests only:
# the repository's `zsh -n` gate reads the exit status, and `-n` still
# applies a top-level `!` to the not-executed command's status (#287).
# Corpus site: none (found while probing #303).
select o in a b c; | cat
select o in a b c; && print x
select o in a b c; || print x
select o in a b c; |& cat
select o in a b c; | cat | wc -l
select o in a; || print x && print y
select o in a b c;
| cat
select o (a b c)|cat
select o (a b c)&&print x
select o (a b c) && print x
select o (a b c) || print x
x=$(select o in a; | cat)
f() { select o in a; | cat }
while true; do select o in a; && break; done
select o in a; | select p in b; | cat
