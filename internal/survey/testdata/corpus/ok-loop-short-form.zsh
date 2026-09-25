# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #211: single-command for, while and until loop forms.
# Preserved as permanent regression coverage.
local -a a=(one two three)
local -i i=0
local t success x l

for t in "${a[@]}"; consume_task "$t"
for i in {$#a..1}; (( i > 100 )) || a[i]=()
for t in "${a[@]}"
  consume_task "$t"
for item (a b c) print -r -- "$item"
for item (a b c); print -r -- "$item"

while (( i < 3 )) (( i++ ))
until (( success )) retry && success="$REPLY"
while [[ -n $x ]] shift
while { read -r l } print -r -- "$l"

f() {
  for x in 1 2; print -r -- "$x"
  while (( i > 0 )) (( i-- ))
  until (( 1 )) break
}
