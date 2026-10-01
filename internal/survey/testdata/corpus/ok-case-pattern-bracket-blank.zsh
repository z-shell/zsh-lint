#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Fixture for #483. Zsh reads a case item that begins with `(` as one word up
# to the matching `)`, so a blank inside a bracket expression of that pattern
# is part of the bracket expression, not a word break.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

TAB=$'\t'
case 'a b' in
  (a[ b]*) print one ;;                 # one
esac
case include${TAB}x in
  (include[ $TAB]*) print two ;;        # two
esac
case ' ' in
  ([ ]) print three ;;                  # three
esac
case ' ' in
  (a|[ ]) print four ;;                 # four
esac
case 'x y' in
  (x[ ]y|z) print five ;;               # five
esac
