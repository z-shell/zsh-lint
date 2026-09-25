# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
# Issue #122: escaped ANSI-C heredoc delimiters
cat <<$'E\x4fF'
body
EOF
