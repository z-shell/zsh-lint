#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #284.
# Alternate Forms For Complex Commands requires an alternate-form test to be
# "suitably delimited, such as by `[[ ... ]]` or `(( ... ))`, else the end of
# the test will not be recognized", so a `{ ... }` group written after `&&` or
# `||` belongs to the condition list rather than opening the body.
#
# The adapter used to read that group as the body, so an ordinary
# `if ... ; then` whose condition held a brace group failed to parse whenever
# the same file also contained an alternate-form construct. Both spellings are
# present here, and every branch is exercised so the fixture proves evaluation
# rather than only parsing.

emulate -L zsh

typeset -g marker=set

# The alternate form the adapter exists for.
if [[ -n $marker ]] {
  print alternate-if
}

if (( 1 )) {
  print alternate-arith
}

# An ordinary `if` whose condition ends in a brace group.
if [[ -n $marker ]] && { true; }; then
  print condition-group
fi

# The same with `||`, and with a negation.
if [[ -z $marker ]] || { true; }; then
  print condition-group-or
fi

if [[ -n $marker ]] && ! { false; }; then
  print condition-group-negated
fi

# Two groups in one condition.
if [[ -n $marker ]] && { true; } && { true; }; then
  print condition-group-twice
fi

# `elif`, which shares the scanner.
if [[ -z $marker ]]; then
  print unreachable
elif [[ -n $marker ]] && { true; }; then
  print condition-group-elif
fi

# The `while` path, which shares it too.
integer count=0
while (( count < 2 )) && { true; }; do
  (( count++ ))
done
print "condition-group-while $count"

# A second delimited test still opens an alternate body.
if [[ -n $marker ]] && [[ -n $marker ]] {
  print alternate-after-and
}

# A condition group followed by the real body brace: native Zsh runs both, so
# the first group is the condition and the second brace is the body.
if [[ -n $marker ]] && { print condition-part; } { print body-part }

print done-284
