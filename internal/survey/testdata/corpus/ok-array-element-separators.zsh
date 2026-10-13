# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Parameters
# Issue #684: in an array assignment a bare `=` is a literal element, and a
# `;` separates elements as a blank or newline does (par_nl_wordlist in
# Src/parse.c reads the list as words and separators). zsh-lint rejected both
# with "array element values must be words".
# Corpus sites: Zsh Completion/BSD/Command/_jail, Completion/Linux/Type/_fuse_values
# and Completion/Unix/Type/_ldap_attributes.
suf=( -qS = )
opts+=(-s , -S =)
local -a l=(x =)
a=(=)
a=([1]=x =)
attrs=(
  automountMapName bindTimeLimit cACertificate;binary cn dc
  defaultServerList
)
b=(a;b)
b=(a ;b)
b=(a; b)
b=(;a;
;b;)
b=(;)
b=([1]=x;[2]=y)
b=(x;$(print a;print b))
f() {
  local -a c=(a;b =)
}
