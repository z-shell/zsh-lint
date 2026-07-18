# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #13: Zsh multi-name for loops are not part of the Bash grammar.
for key value in a 1 b 2; do
  print -r -- "$key=$value"
done
