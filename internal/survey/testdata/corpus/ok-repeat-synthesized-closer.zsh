# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #300: a repeat sublist ending in a closer another adapter synthesized.
# Preserved as permanent regression coverage.
repeat 3 for i (a b) { print -r -- "$i" }
print -r -- after-for
repeat 2 if (( 1 )) { print -r -- hi }
print -r -- after-if
repeat 2; for i (a b) { print -r -- "$i" }
print -r -- after-separator
repeat 3 for i (a b) { print -r -- "$i" }
