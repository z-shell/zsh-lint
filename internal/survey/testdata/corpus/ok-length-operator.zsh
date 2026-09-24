#!/usr/bin/env zsh
# An expansion operator under a length prefix (#373).
#
# Zsh applies `#` to the RESULT of the rest of the expansion (zshexpn,
# Parameter Expansion), so the length and the operator compose. mvdan/sh
# rejects the combination outright, because in POSIX and bash `${#name}`
# admits nothing after the name.
#
# Every line prints, so a wrong tree shows up as a wrong value rather than
# only as a parse difference.

typeset -a a
a=( x y z )
typeset -A Z
Z=( one 1 two 2 )
profile=beta
typeset -a profiles
profiles=( alpha beta gamma )

# The reported gap: the count of the elements that do not match a pattern.
print -r -- ${#a[@]:#x}
print -r -- ${#a:#x}

# The zi lib/zsh/install.zsh idiom this gap blocks.
(( ${#profiles[@]:#$profile} > 0 )) && print -r -- other-profiles-exist

# The length applies after the operator, so these differ.
print -r -- ${#a[@]} ${#a[@]:#x}

# Every other operator composes with a length prefix too.
print -r -- ${#a//x/y}
print -r -- ${#a/x/y}
print -r -- ${#a:-fallback}
print -r -- ${#a:+alternate}
print -r -- ${#a#x}
print -r -- ${#a%z}
print -r -- ${#a[@]//x/y}

# Slices and bare modifiers are NOT covered: they are masked into a path
# that is more permissive than Zsh, so the adapter refuses them rather than
# spread that permissiveness. `${#a:1}` stays rejected, as it is on main.

# A subscript flag before the operator.
print -r -- ${#a[(r)x]:#nomatch}

# An associative name, and a length inside arithmetic.
print -r -- ${#Z[@]:#1}
print -r -- $(( ${#a[@]:#x} + 1 ))

# Inside double quotes, and nested in another expansion.
print -r -- "${#a[@]:#x}"
print -r -- ${unset_name:-${#a[@]:#x}}

# The nested spelling, which already parsed, still means the same thing.
print -r -- ${#${a[@]:#x}}
