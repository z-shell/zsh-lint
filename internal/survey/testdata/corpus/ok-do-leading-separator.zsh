# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #238: a loop body may begin with a separator directly after `do`.
# The parser reads `do;` as an empty body and then demands `done`; the front
# end masks that one separator and verifies the loop's `do` in the tree.
# Corpus site: upstream Zsh's shipped example zshrc, vendored in z-shell/zpmod.
freload() { while (( $# )); do; unfunction $1; autoload -U $1; shift; done }
while (( $# )); do; shift; done
for x in a b; do; print $x; done
until (( $# )); do; shift; done
select o in a b c; do; print $o; break; done
for ((i = 0; i < 2; i++)); do; print $i; done
while true; do;break; done
while true; do ; break; done
while true; do	;break; done
while true; do; ; break; done
while true; do
; break; done
while true; do;
; break; done
while true; do # comment
; break; done
while true; do; # comment
break; done
while true; do; while true; do; break; done; break; done
while true; do; break; done; while true; do; break; done
while true; do; break; done | cat
for x in do; do; print $x; done
while print do; do; break; done
while true; do; f() { ; }; done
while true; do; cat <<EOF2
do;
EOF2
break; done
while true; do; print 'do; x' "do; y"; break; done
while true; do; done
while true; do ;done
while true; do; print $(while true; do; break; done); break; done
