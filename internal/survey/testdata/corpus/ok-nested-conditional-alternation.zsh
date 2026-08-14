# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #112: zsh-lint locally preserves nested pattern-alternation groups on
# conditional pattern operands while retaining original AST text and positions.
line=foo
[[ $line == ((a|b)|c) ]] && print matched
print `[[ $line == ((a|b)|c) ]]`suffix
print "`[[ $line == ((a|b)|c) ]]`suffix"
print `print \`[[ $line == ((d|e)|f) ]]\``
print `[[ $line == ((a|b)|c) ]]`