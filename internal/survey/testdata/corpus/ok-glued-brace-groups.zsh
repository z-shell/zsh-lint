# vim: set filetype=zsh:
# Issue #276: brace groups glued to command words, in test-fixture placement.
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
{print hi}
{ print hi}
if true; then {print hi} fi
repeat 2 {print hi}
glued_group() {print hi}
{print ${glued_value:-hi}}
{print a{b,c}}
{print $(print hi)}
select glued_choice in a; {break}
