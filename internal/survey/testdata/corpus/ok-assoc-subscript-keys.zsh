# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issues #61 and #129: native-Zsh bare associative keys may begin with a dot
# or contain punctuation that mvdan/sh otherwise treats as arithmetic syntax.
print ${functions[.foo]}
ZI[annex-before-load:new-@]=value
