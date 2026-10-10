#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Positional-Parameters
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-integer
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-float
# Track 2.4 of #612: numeric parameter names are assignments, and integer
# and float are declarations. Top-level test fixture, native Zsh 5.9.2.
0=${ZERO:-${(%):-%N}}
1=value
12="two words"
1+=suffix
integer n=1 m=2
float -F 2 fraction=1.5
integer values=(1 2)
float fractions=(1.5 2.5)
{ integer count=1 }
{ float ratio=1.5 }
integer() { :; }
float () { :; }
print -r -- 0=value
