#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #466.
# A dangling `&&` or `||` ends its list when only list trivia stands between
# it and the closer: a `\` line continuation, blank lines, or a comment.
# F-Sy-H functions/fsh_theme writes `print ... || \` directly before `else`.
#
# Every operator below is closed by a closer; across a bare newline an
# operator would take the next statement as its right operand instead.

emulate -L zsh

if true; then
  print else-after-continuation || \
else
  print not-reached
fi

if false; then
  print not-reached
elif true; then
  print elif-branch && \
elif true; then
  print not-reached
fi

if true; then
  print fi-after-continuation || \

fi

while false; do
  print not-reached || \
done

h() {
  print brace-after-continuation && \
}

g() { print brace-after-comment || # trailing comment
}

f() {
  print comment-line-before-brace &&
  # a comment line between the operator and the closer
}

( print subshell-after-continuation || \
)

if print then-after-continuation || \
then
  print then-branch
fi

h
g
f

print done-466
