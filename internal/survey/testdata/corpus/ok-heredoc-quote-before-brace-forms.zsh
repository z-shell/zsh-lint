#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Fixture for #429. A here-document body is text read as the command's input,
# so an odd quote in it opens nothing; the brace-form commands after it are
# ordinary alternate forms. The adapters' source scanners read the body as
# command text before #429 and lost every later brace-form site.
#
# Every form below passes `zsh -f -n` and runs to completion under `zsh -f`.

x=1

cat <<'EOF' >/dev/null
it's
EOF
if [[ -n $x ]] { print if-after-quoted-delimiter }

cat <<EOF >/dev/null
say "hi
EOF
while [[ -n $x ]] { print while-after-unquoted-delimiter; x= }

cat <<-EOF >/dev/null
	'
	EOF
for y (a b) { print for-after-tab-stripped $y }

cat <<A <<'B' >/dev/null
'
A
"
B
for p q (1 2) { print multi-name $p $q }

cat <<< "'" >/dev/null
if [[ -z $x ]] { print if-after-here-string }

: $(( 1 << 2 ))
(( z = 3 << 1 ))
if (( z == 6 )) { print if-after-arithmetic-shift }
