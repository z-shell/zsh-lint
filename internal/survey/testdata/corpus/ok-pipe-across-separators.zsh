#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #553.
# Native Zsh reads a `|` or `|&` across any `;` and newline separators:
# `par_pline` skips every separator token after the pipe before it reads the
# next command, so `print a | ; cat` is the pipeline `print a | cat`.
# Every line below prints through the pipe it continues.

emulate -L zsh

print same-line | ; cat

print own-lines |
;
cat

print stderr-pipe |& ; cat

print two-separators | ; ; cat

print comment-between | ;

# the pipe reads past this comment
cat

print chained | ; cat | ; cat

h() {
  print in-function |
  ;
  cat
}
h

{ print in-group | ; cat; }

( print in-subshell | ; cat )

print "$(print in-substitution | ; cat)"

if true; then
  print in-then | ; cat
fi

case pipe in
  pipe) print in-case | ; cat ;;
esac

print done-553
