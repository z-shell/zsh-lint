# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Comments
# Regression fixture for #643. A comment is a word that begins with `#`, so a
# `#` glued to word text, as in `${#x}`, `${x}#y` or `\{#y`, is part of that
# word, and so is a `#` inside a `${...}` even after a blank there. The scan
# that closes a brace block after a declaration with no separator before `}`
# took each such `#` for a comment and missed the closing `}`.
{ local n=${#x} }
{ local n=${#${x}} }
{ local n=${x}#y }
{ local n=\{#y }
f() { local n=${#x} }
{ local n=${y:- #a} }
{ local n=${(j: #:)y} }
{ print -r -- ${#y}; local x=1 }
{ local x=1 } # ${#y} }
