# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Flags
# Issue #277: the expression after ',' in a flagged subscript holds a bracket
# expression or starts with '--'; mvdan/sh reads it as arithmetic, where '[',
# '--' and '^' are operators, while Zsh reads it by the parameter's type.
local -A ___opt_map
local -a a
local opt
print -r -- ${___opt_map[(r)a,[^:]##]}
print -r -- ${___opt_map[(r)a,--[^:]##]}
print -r -- ${___opt_map[(r)a,b[^:]]}
print -r -- ${___opt_map[(r)a,[a,b]]}
print -r -- ${___opt_map[(r)a,\]x]}
print -r -- ${___opt_map[(rn:2:)a,[^:]]}
print -r -- ${___opt_map[(r)a,--[^:]##]:-none}
print -r -- ${a[(r)y,--x]}
___opt_map[(r)a,[^:]##]=1
local msg=${___opt_map[$opt]#*:} txt=${___opt_map[(r)opt_$opt,--[^:]##]}
print -r -- ${___opt_map[(r)a,[^:]"x"]}
print -r -- ${___opt_map[(r)a,[^:]'x y']}
print -r -- ${___opt_map[(r)a,[^:]"x,y"]}
print -r -- ${___opt_map[(r)a,[^:]"x\"y"]}
print -r -- ${___opt_map[(r)a,[^:]$'x\'y']}
print -r -- ${___opt_map[(r)a,"[^:]"]}
print -r -- ${___opt_map[(r)a,--"[^:]"]}
print -r -- ${___opt_map[(r)a,[^:]'x\]y']}
print -r -- ${___opt_map[(r)a,[^:]'x\\\]y']}
