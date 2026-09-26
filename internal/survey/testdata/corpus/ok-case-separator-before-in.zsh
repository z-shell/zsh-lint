# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Issue #485: a separator between the case word and `in` is accepted by Zsh.
# Preserved as permanent regression coverage.

x=val
case $x; in
  val) : ;;
esac

case $x; ; in
  val) : ;;
esac

case $x ;
in
  val) : ;;
esac

case $x;
in
  val) : ;;
esac
