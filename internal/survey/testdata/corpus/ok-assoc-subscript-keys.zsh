# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Subscripts
# Issues #61, #129, and #194: native-Zsh bare associative keys may begin with a dot
# or '@', or contain punctuation that mvdan/sh otherwise treats as arithmetic syntax.
print ${functions[.foo]}
ZI[annex-before-load:new-@]=value
print ${functions[@zi-register-annex]}
(( ${+functions[@zi-register-annex]} ))
