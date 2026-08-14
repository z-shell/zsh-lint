# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #12: Zsh try/always blocks are valid complex commands but are not
# parsed by the current mvdan/sh LangZsh front end.
{
  :
} always {
  :
}
