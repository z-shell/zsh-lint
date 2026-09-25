# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
# Issue #216: the unconditional assignment operator ${name::=word}.
# Preserved as permanent regression coverage.
local x y=value name=x
local -A a
local -a arr

print ${x::=value}
print ${x::=$y}
print ${x::="${(kv)a[@]}"}
print ${x::=}
print "${x::=quoted}" ${arr[1]::=first}
print ${${x::=inner}#in} ${y:+${x::=${y:+${x:+nested}}}} ${x:=conditional}
# zsh -f -n 5.9.2 rejects a (P)-flagged assignment to a name an earlier
# assignment expansion already used, so this row keeps its own name.
: ${(PA)name::="${(kv)a[@]}"}
