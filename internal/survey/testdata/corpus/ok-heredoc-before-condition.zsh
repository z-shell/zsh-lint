# vim: ft=zsh
# Issue #701: retain an earlier document across a nested-pattern condition.
# Placement: top-level test fixture; never execute this source.
# Manual: https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection-1
cat <<EOF; [[ $line == ((a|b)|(x)) ]]
body
EOF
echo after
