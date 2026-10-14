# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
# Issue #686: Zsh ends a here-document that no line closes at the end of the
# file. One whose body has not started ends at a closing backquote, and at
# the parenthesis closing its command substitution, so the next line is a
# command. zsh-lint reported "unclosed here-document" at end of file and at
# the backquote, and read the lines after `$(cat <<EOF)` as its body.
x=`cat <<EOF`
print -r -- $(cat <<EOF) $x
cat <<EOF
hello
