# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #241: for (( [expr1] ; [expr2] ; [expr3] )) sublist, the alternate arithmetic for form.
# Preserved as permanent regression coverage.
for (( i = 1; i < 3; i++ )) {
  print -r -- "$i"
}
for (( ; ; )) {
  break
}
for (( i = 1; i < 3; i++ )) print -r -- "$i"
for (( i = 1; i < 3; i++ )); print -r -- "$i"
for (( i = 1; i < 3; i++ ))
  print -r -- "$i"
f() {
  for (( i = 0; i < 2; i++ )) { print -r -- "in-func $i" }
  for (( i = 0; i < 2; i++ )) print -r -- "in-func-single $i"
}
for (( i = 1; i < 3; i++ )) cat <<EOT
$i
EOT
print -r -- done
