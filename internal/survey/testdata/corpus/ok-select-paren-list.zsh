# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #303: the parenthesized list form of select, `select name ( word ... )`
# followed by a sublist, a `{ list }` brace body, a `do list done` body, a
# separator then a body, or nothing (empty body), is native-valid Zsh (par_for
# reads it through the same INPAR branch as `for name ( word ... )`) but rejected
# by zsh-lint with "`select foo` must be followed by `in`, `do`, `;`, or a newline".
# Corpus site: none (split from #212 during implementation).
select o (a b c) break
select o (a b c); break
select o (a b c) { print $o; break }
select o (a b c) { print $o } && print x
select o (a b c) do print $o; break; done
select o (a b c); do break; done
select o (a b c) print -r -- "$o" | cat
select o ( a b c ) break
select o (a
  b
  c) break
select o ("a b" $(print c) 'd' ${x:-e}) break
select o (')' "x") break
select o (a b c\
) break
f() { select o (a b c) }
g() {
  select o (a b c) break
}
while true; do select o (a b c) break; done
select o1 (a b) break; select o2 (c d) break
select o (a b) select p (c d) break
select o () break
select o in a b c; break
select o in a b c; do break; done
select o (a b c)
