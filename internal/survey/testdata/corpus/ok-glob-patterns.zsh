# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Recursive-Globbing
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Regression (was gap #16): filename-generation patterns parse under LangZsh;
# minimized from zunit build.zsh.
cat src/**/(^zunit).zsh
