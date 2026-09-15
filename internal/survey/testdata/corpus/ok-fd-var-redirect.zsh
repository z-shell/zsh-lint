# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #105: the {varname} redirection form opens a new file descriptor and
# stores its number in varname (zshmisc, REDIRECTION). This is dynamic
# file-descriptor allocation, not the MULTIOS option. It is valid Zsh; LangZsh
# rejected it as bash-only before mvdan/sh v3.14.1. Minimized from real-code
# socket handling.
exec {fd}>&-
