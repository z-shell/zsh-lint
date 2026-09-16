# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #230: a brace-form if body containing ${#name} broke the alternate-if
# scanner, which read the `#` as a comment and skipped the closing brace.
# Preserved as permanent regression coverage.
local -a frames
frames=( a b c )
integer cur=0
if (( SECONDS >= 1 )) {
  (( cur = (cur+1) % (${#frames}+1-1) ))
}
if (( cur )) { print ${#frames} } else { print ${#frames[1]} }
if [[ -n $frames ]] { print "${#frames}" $#frames ${frames:#a} ${${frames[1]}#a} }
