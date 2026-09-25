#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Fixture for #452. Zsh reads a case pattern that begins with `(` as one word.
# A leading group glued to more pattern text is part of the pattern, not the
# optional opener, and `((` at the start is the opener followed by a group.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

case xy in
  (x)y) print one ;;                    # one
esac
case z in
  ((x|y)|z) print two ;;                # two
esac
case y in
  (x|y)) print three ;;                 # three
esac
case xy in
  (x)(y)) print four ;;                 # four
esac
case xy in
  ((x)y) print five ;;                  # five
esac
case z in
  (x)y|z) print six ;;                  # six
esac
case c in
  (a|b)|(c)) print seven ;;             # seven
esac
case x in
  (x) print eight ;;                    # eight
esac
