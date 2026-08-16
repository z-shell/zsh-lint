# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #104: parameter expansion glob-toggle ${~spec} does not
# parse under LangZsh; minimized from real-world hostname glob matching.
pattern='foo*'
str='foobar'
[[ "$str" == ${~pattern} ]] && print matched
