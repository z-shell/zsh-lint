# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #105: {varname} dynamic file-descriptor redirection (MULTIOS)
# incorrectly rejected as bash-only under LangZsh; minimized from real-code socket handling.
exec {fd}>&-
