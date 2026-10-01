#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Fixture for #481. Zsh reads a `((` that continues a word as a glob group
# whose first byte opens a nested group, not as an arithmetic command, so
# `x((b)c)` is one pattern word in a case item and in any other word.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

case xbc in
  x((b)c)) print one ;;                 # one
esac
case xb in
  x((b))) print two ;;                  # two
esac
case y in
  x((b)c)|y) print three ;;             # three
esac
case xbcd in
  (x(((b)c)d)) print four ;;            # four
esac
setopt extendedglob
print -r -- ${${:-xbc}:#x((b)c)}five    # five
