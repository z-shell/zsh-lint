#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Fixture for #435. A here-string `<<< word` is a word, not a here-document:
# the try/always adapter's scanner read `<<<` as a here-document operator,
# failed to read its delimiter, and gave up on the whole block.
#
# Every form below passes `zsh -f -n` and runs to completion under `zsh -f`.

x=1
{
  :
} always {
  cat <<< "'" >/dev/null
  if [[ -n $x ]] { print -r -- after-here-string-in-always }
}

{
  cat <<< "'" >/dev/null
} always {
  print -r -- here-string-in-try
}

{ cat <<< "it's" >/dev/null } always { print -r -- one-line }
