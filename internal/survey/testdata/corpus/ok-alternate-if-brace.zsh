# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #125: alternate brace-form Zsh if commands (if [[ ... ]] { ... }).
# Preserved as permanent regression coverage.
if [[ 1 ]] {
  :
}
