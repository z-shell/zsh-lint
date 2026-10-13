# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion-Flags
# Regression (#644): a `)` inside a delimited flag argument does not end the
# flags group. Function-body placement, where `zsh -f -n` checks only the
# lexer (#287); the shapes come from powerlevel10k wizard.zsh, Oh My Zsh
# theme_chooser.sh, and Zsh's multicomp, _pids and _zle.
flag_args() {
  local -a words=( a b ) desc=( 'PID TTY' )
  local -i cols=80 used=2
  print -r -- ${(pl:$(((cols - used) / 2)):: :)}
  print -r -- "${(l.((${cols}-${#words}-5))..-.)}"
  print -r -- "[${(j(][))${(s())words:l}}]*"
  print -r -- ${(l.${#desc[1,(r)PID]}..?.)}
  print -r -- "(${(j(|))words})"
}
