#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html#Conditional-Expressions
# Fixture for #440. A case arm may start its pattern with the optional `(`.
# The nested-pattern scanner counted that `(` as a pattern group, so the `)`
# after the pattern never ended it and a `[[ ... ]]` nested group in the arm
# body was not seen. Zsh reads the leading `(` as the opener when its `)` is
# followed by a blank, `;` or the end of input, and as a group when more
# pattern follows (`(x|y))`).
#
# Every arm below passes `zsh -f -n` and runs under `zsh -f`, printing its label.

x=a
case x in
  (x) [[ $x == (a|(b|c)) ]] && print -r -- same-line ;;
esac
case x in
  (x)
    [[ $x == (a|(b|c)) ]] && print -r -- next-line
    ;;
esac
case y in
  (x) : ;;
  (y) [[ $x == (a|(b|c)) ]] && print -r -- second-arm ;;
esac
case y in
  (x|y)) [[ $x == (a|(b|c)) ]] && print -r -- grouped-pattern ;;
esac
