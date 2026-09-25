# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
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
# Bytes inside an expansion are word bytes: `## ##` holds no comment, so the
# body's closing brace and the else chain stay visible to the scanner
# (minimized from z-a-meta-plugins _z_a_meta_plugins_before_load_handler).
local -A ZI
ZI[annex-before-load:new-@]='a b '
if (( cur )) {
  ZI[annex-before-load:new-@]=${ZI[annex-before-load:new-@]## ##}
} else {
  ZI[annex-before-load:new-@]=${${ZI[annex-before-load:new-@]## ##}%% %%}
}
