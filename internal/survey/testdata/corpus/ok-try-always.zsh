# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Issue #12: Zsh try/always blocks ({ ... } always { }).
# Preserved as permanent regression coverage.
{
  :
} always {
  :
}
