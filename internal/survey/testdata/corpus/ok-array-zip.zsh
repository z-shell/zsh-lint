# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Regression fixture for #671. `${name:^arrayname}` and `${name:^^arrayname}`
# zip two arrays; they are expansion operators, not the start of a slice.
a=(1 2 3 4)
b=(x y)
print -r -- ${a:^b} ${a:^^b} "${a:^b}" ${(j:,:)a:^b}
c=(${a:^^b})
zip() {
  print -r -- ${tmatch:^^tdisp} ${serv:^desc}
}
