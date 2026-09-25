# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #212: select name [in word ...] term sublist, the short form of select.
# Preserved as permanent regression coverage.
select o in a b c; break
case $o in
  (a) print -r -- first ;;
  (*) print -r -- other ;;
esac
select o in "a b" $(print c) 'd'
  print -r -- "$o" && break
select o; break
select o break
f() {
  select o in a b; print -r -- "$o" | cat
  select o in a b; break; print -r -- after
}
select o in a b; cat <<EOT
$o
EOT
print -r -- done
