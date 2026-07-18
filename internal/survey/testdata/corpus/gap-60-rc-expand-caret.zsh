# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #60: rc-expand caret ${^spec} (zshexpn, RC_EXPAND_PARAM) does not
# parse under LangZsh; minimized from zsh-fancy-completions .man_glob.
print -- ${^manpath}
