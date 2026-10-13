#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Fixture for #578. Zsh reads a case item that begins with `(` as one word up
# to the matching `)`, so a blank or newline anywhere in that group is pattern
# text. When the group is the whole pattern, the blanks next to its `(`, its
# `)` and each `|` are dropped; a group glued to more text keeps them all.
#
# Every arm passes `zsh -f -n` and runs under `zsh -f`; the expected output is
# noted on each line.

case 'a b' in
  (a b) print one ;;                    # one
esac
case 'ok # SKIP x' in
  (*# SKIP*) print two ;;               # two
esac
case '# environment' in
  (\# Not a target*|\# environment*) print three ;;   # three
esac
case 'exec foo' in
  (''(x|exec) *) print four ;;          # four
esac
case b in
  ( a | b ) print five ;;               # five
esac
case 'a ' in
  (a | b)) print six ;;                 # six
esac
case $'a\nb' in
  (a
b) print seven ;;                       # seven
esac
