# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Baseline: plain POSIX-compatible constructs the front end must always parse.
say() { print -r -- "$1"; }
for x in a b c; do say "$x"; done
