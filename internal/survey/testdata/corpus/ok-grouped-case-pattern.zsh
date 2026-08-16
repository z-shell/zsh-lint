# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #123: grouped Zsh case patterns close before the case-arm terminator.
# Preserved as permanent regression coverage.
case x in
  (x|y)) : ;;
esac
