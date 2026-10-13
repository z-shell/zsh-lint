# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
# Regression fixture for #672. Zsh substitutes expansions before it evaluates
# arithmetic, so an expansion at a blank may supply an operator or operand.
op=+
print -r -- $(( 1 $op 2 )) $(( (1) $op (2) ))
(( x = 1 $op $op 2 ))
count() {
  print -r -- $(( $#dirstack $linepath )) $(( $_base $1 ))
}
