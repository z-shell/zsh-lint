# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Issue #297: an `if` branch may begin with a separator directly after `then`
# or `else` (or after `elif ...; then`). The parser reads `then;` or `else;`
# as an empty branch; the front end masks that one separator and verifies
# the `if` construct in the tree.
# Corpus site: none (found while probing the #238 shape).
f() { if true; then; print x; fi }
if true; then; print x; fi
if true; then x; else; y; fi
if true; then; x; else; y; fi
if true; then x; elif false; then; y; fi
if true; then; x; elif false; then; y; else; z; fi
if true; then;print x; fi
if true; then ; print x; fi
if true; then	;print x; fi
if true; then; ; print x; fi
if true; then
; print x; fi
if true; then;
; print x; fi
if true; then # comment
; print x; fi
if true; then; # comment
print x; fi
if true; then x; else ; print y; fi
if true; then x; else	;print y; fi
if true; then x; else; ; print y; fi
if true; then x; else
; print y; fi
if true; then x; else # comment
; print y; fi
if true; then x; else; # comment
print y; fi
if true; then; if true; then; print x; fi; fi
while true; do; if true; then; print x; fi; break; done
if true; then; while true; do; break; done; fi
if true; then; print x; fi; if true; then; print y; fi
if true; then; print x; fi && print z
if true; then; print x; fi | cat
for x in then; do print $x; done
if print then; then; print x; fi
if true; then; print 'then; x' "then; y"; fi
if true; then; cat <<EOF2
then;
EOF2
print x; fi
if true; then; fi
if true; then ;fi
if true; then; else; fi
if true; then ; else ; fi
if true; then; print $(if true; then; print x; fi); fi
