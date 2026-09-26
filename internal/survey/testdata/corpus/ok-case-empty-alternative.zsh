#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Glob-Operators
# Fixture for #396. Native Zsh accepts a case pattern with an empty alternative.
# The empty alternative matches the empty string.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

case "" in
  (|a) print one ;;                     # one
esac
case "" in
  (a|) print two ;;                     # two
esac
case "" in
  |a) print three ;;                    # three
esac
case "" in
  a|) print four ;;                     # four
esac
case "" in
  |a|) print five ;;                    # five
esac
case "" in
  (a||b) print six ;;                   # six
esac
case "" in
  (|) print seven ;;                    # seven
esac
case "" in
  ||) print eight ;;                    # eight
esac
case "" in
  (|a|) print nine ;;                   # nine
esac
case "" in
  (|https|git|http|ftp|ftps|rsync|ssh) print ten ;; # ten
esac
case "" in
  (a||) print eleven ;;                 # eleven
esac
case "" in
  |) print twelve ;;                    # twelve
esac
case "" in
  (||) print thirteen ;;                # thirteen
esac
case "" in
  a||b) print fourteen ;;               # fourteen
esac
case "" in
  |a|b) print fifteen ;;                # fifteen
esac
case "" in
  (a|b|) print sixteen ;;               # sixteen
esac
case $1 in (|a) print hit ;; esac
