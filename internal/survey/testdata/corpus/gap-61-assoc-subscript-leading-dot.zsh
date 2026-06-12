# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #61: associative-array subscripts beginning with a dot (zshparam,
# Array Subscripts) do not parse under LangZsh; minimized from the
# ${+functions[.zsh-eza]} probe in zsh-eza.plugin.zsh.
print ${functions[.foo]}
