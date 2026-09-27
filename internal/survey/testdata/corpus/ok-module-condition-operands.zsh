# -*- mode: sh; sh-shell: zsh; -*-
# Issue #484: module conditions are parsed in this never-called function.
# Unloaded module conditions fail at runtime, so do not call this function.
# Manual: https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html
module_condition_operands() {
  builtin emulate -L zsh
  [[ -prefix - ]]
  [[ -prefix -x ]]
  [[ ! -prefix - ]]
  [[ -prefix a ]]
  [[ -after a b ]]
  [[ -between a b c ]]
  [[ -foo a b c d ]]
  [[ -foo a ]] && print y
  [[ a -foo b ]]
  [[ -prefix - && -n x ]]
  [[ -prefix - || -z x ]]
  [[ ( -prefix - ) ]]
  [[ -n a b ]]
  [[ -z ]]
  [[ -n ]]
  [[ -f a b ]]
  [[ -prefix a b c ]]
  [[ -- a ]]
  [[ -1 a ]]
  [[ -a-b c ]]
  [[ -prefix $x ]]
  [[ -prefix "-" ]]
  [[ -foo a -bar b ]]
  [[ -foo a && b ]]
  [[ -foo ]]
  [[ -prefix ]]
  [[ a -nt b ]]
  [[ -z a ]]
  [[ a == b ]]
  # A glob group in an operand nests bare parentheses.
  [[ -foo a ( b (c) ) ]]
  [[ -foo a(b(c)d) e ]]
  [[ a -foo b(c(d)e) ]]
  [[ -n a(b(c)d) ]]
  [[ -foo a ( b <1-5> ) ]]
}
