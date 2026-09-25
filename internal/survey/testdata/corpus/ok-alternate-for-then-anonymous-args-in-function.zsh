# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Functions.html#Anonymous-Functions
# Issue #255: an alternate `for name ( words ) { ... }` loop earlier in the
# file must not break a later anonymous function call with arguments inside
# a named function.
# Preserved as permanent regression coverage.
for x ( a ) {
}
f() {
  () { x; } y
}
