# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Functions.html
# Issue #234: function definitions whose name contains a parameter expansion.

cur=foo
cur_widget=bar

_w_${cur}() { :; }
_w_$cur() { :; }
function _w_${cur} { :; }
_fsh_widget_${cur_widget}() { :; }
