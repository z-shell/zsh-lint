# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #209: a reverse-subscript pattern containing `/` parses under
# mvdan/sh v3.14.1 (`FlagsArithm R`, pattern word); v3.13.1 read the
# subscript as arithmetic. Minimized from z-shell/zi zi.zsh:445.
fpath_elements=( ${fpath[(R)$PLUGIN_DIR/*]} ${fpath[(R)${PLUGIN_DIR:A}/*]} )
