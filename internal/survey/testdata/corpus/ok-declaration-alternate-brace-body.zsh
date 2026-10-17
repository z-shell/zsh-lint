# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Top-level syntax fixture.
# Issue #649: declarations terminate at the alternate body's closing brace.
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
if [[ -n x ]] { local u=1 }
for i in 1; { local u=1 }
