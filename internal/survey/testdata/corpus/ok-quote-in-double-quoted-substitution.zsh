#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
# Fixture for #393. Inside a double-quoted string, `$(...)`, `${...}` and
# backquotes start their own quoting context, so an odd quote in a nested
# string is a literal byte there. Every alternate form after such a string is
# still found. Minimized from z-shell/zi lib/zsh/install.zsh:455.
#
# Every line passes `zsh -f -n`, and the file runs to completion under
# `zsh -f`.
x=
print -r -- "$(print -r -- "it's")"
for n ( a b ) { print -r -- $n }
print -r -- "$(print -r -- '"')"
select n ( a ) { break } </dev/null
print -r -- "`print -r -- "it's"`"
if (( 1 )) { print -r -- if-body }
print -r -- "${x:-"it's"}"
n=0
while (( n < 1 )) { (( n++ )) }
print -r -- "$(print -r -- "$(print -r -- "it's")")"
repeat 1 do print -r -- repeat-body; done
print -r -- "${x:-{}"
for n ( d ) { print -r -- $n }
print -r -- "$(print -r -- ${x:-$'it\'s'})"
if (( 1 )) { print -r -- ansi-c-body }
print -r -- "$([[ a == (#b)(*) ]] && print -r -- "it's")"
for n ( e ) { print -r -- $n }
print -r -- "$(cat <<<"it's")"
for n ( f ) { print -r -- $n }
print -r -- "$(# a comment right after the opener )
print -r -- "it's")"
for n ( g ) { print -r -- $n }
print -r -- "$(print -r -- case "it's")"
if (( 1 )) { print -r -- case-argument-body }
print -r -- "$( (( 1 << 2 )) && print -r -- "it's" )"
for n ( h ) { print -r -- $n }
print -r -- "$( (# a comment in a subshell )
print -r -- "it's") )"
for n ( i ) { print -r -- $n }
print -r -- "$(a=( (#i)x(N) ); print -r -- "it's")"
for n ( j ) { print -r -- $n }
print -r -- "$(typeset -a a=(# a comment in a declared array
  x ); print -r -- "it's")"
for n ( k ) { print -r -- $n }
g() {
  print -r -- "$(print -r -- "it's")"
  for n ( c ) { print -r -- $n }
}
g
{ print -r -- try-body } always { print -r -- always-body }
