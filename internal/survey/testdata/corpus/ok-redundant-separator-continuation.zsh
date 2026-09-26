#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
# Fixture for #467. A `\` line continuation is removed before the line is
# read, so a `;` on the next line still stands in command position and is a
# redundant separator, as in `print a; ;`. After a dangling `||` Zsh skips
# the `;` too, so the next statement becomes the right operand: running this
# prints one, two, four and done_marker, and `three` does not run because
# `print two` succeeded. The parsed tree is the same `print two || { ... }`.
print one; \
;
print two || \
;
{ print three || \
; }
print four; \
\
;
print done_marker
