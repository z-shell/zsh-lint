# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Rules
# Issue #215: a second subscript applied to the result of the first, ${name[a][b]}.
# Preserved as permanent regression coverage.
local -a a argv line
local -A sice ZI
local argi=1 i=2

print ${a[1][2]}
print ${a[b][1,50]}
print "${sice[ps-on-unload][1,50]}"
print "${ZI[col-info]}${sice[ps-on-unload][1,50]}${sice[ps-on-unload][51]:+…}${ZI[col-rst]}"
argv[$argi]=(-n ${argv[$argi][2,-1]})
print ${a[b][-1]}
print ${a[b][$i]}
print ${a[b][${i}]}
print ${a[b][$line[1]]}
print ${(f)a[b][1]}
print ${#a[b][1]}
print ${a[1,2][3]}
print ${a[1][2][3]}
print ${a[b][(r)x*]}
print ${a[b][1]:-none}
