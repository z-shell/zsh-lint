# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Issue #207 follow-up: a brace-form if ends at the newline after its closing
# brace, so the classic else and elif on the next lines belong to the
# enclosing if. Minimized from z-shell/zi zi.zsh:523-558.
for func; do
  if [[ ${ZI[NEW_AUTOLOAD]} = 2 ]]; then
    retval=$?
  elif [[ ${ZI[NEW_AUTOLOAD]} = 1 ]]; then
    if (( ${+opts[(r)-C]} )) {
      retval=$?
    } else {
      retval=$?
    }
  else
    retval=$?
  fi
done
