# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Regression (was gap #12): `}` closing a brace body without a preceding
# terminator parses under LangZsh; minimized from .completion-prediction.
f() { print -r -- hi }
