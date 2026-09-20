# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #251: multi-name function definitions name1 name2 () { list }.
# Preserved as permanent regression coverage.
# z-shell/zpmod vendor/zsh/Test/ztst.zsh:310 defines ZTST_prep and
# ZTST_clean with one body; every listed word gets the same function.
a b () { : }
a b() { : }
ZTST_prep ZTST_clean () {
  print prep
}
if true; then c d () { : }; fi
time e f () { : }
g h () print hi
i j () # comment before the body
{
  print ij
}
k-l m.n:o () { : }
a; b; ZTST_prep; ZTST_clean; c; d; e; f; g; h; i; j; k-l; m.n:o
