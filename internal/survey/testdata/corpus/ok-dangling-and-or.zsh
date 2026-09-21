#!/usr/bin/env zsh
# Fixture for #331.
# Native Zsh accepts `&&` and `||` as the last token of a list: the operator has
# no right operand, and the left one runs exactly as a bare statement would.
#
# An operator is only dangling at a list TERMINATOR. Across a bare newline it
# takes the following statement as its right operand and short-circuits it, so
# every operator below is closed by `;`, a closer, or end of file.

emulate -L zsh

h() { print in-function &&
}

g() { print same-line && }

( print in-subshell &&
)

if true; then
  print in-then &&
fi

while false; do
  print in-loop &&
done

case dangling in
  dangling) print in-case &&
    ;;
esac

h
g

print top-level-and &&
;
print top-level-or ||
;

print done-331
