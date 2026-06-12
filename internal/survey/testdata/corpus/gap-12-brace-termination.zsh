# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #12: Zsh allows `}` to close a brace body without a preceding
# terminator; minimized from zsh-fancy-completions .completion-prediction.
f() { print -r -- hi }
