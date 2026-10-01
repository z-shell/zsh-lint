#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Globbing-Flags
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Fixture for #482. Zsh reads a case item that begins with `(` as one word,
# so a `#` glued to that opener or to a `|` inside it is pattern text, as in
# a leading globbing flag, not a comment. A `#` glued to the `)` of a leading
# group is the glob operator.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

setopt extended_glob
case X in
  (#i)x) print one ;;                   # one
esac
case --adj in
  (#b)(--adj)) print two $match[1] ;;   # two --adj
esac
case --adjustment=-5 in
  (#b)(--adjustment)(=(-|+|)[0-9]#|)) print three $match[2] ;; # three =-5
esac
case y in
  (#i)x|y) print four ;;                # four
esac
case Ab in
  (#i)a(#I)b) print five ;;             # five
esac
case xxx in
  (x)#) print six ;;                    # six
esac
case x in
  ((#i)X) print seven ;;                # seven
esac
