# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #301: the brace-body form of select, `select name [in word ...] term { list }`
# (term is `;` or a newline; with no `in` list the term may be absent:
# `select o { list }`), is native-valid Zsh but rejected by zsh-lint. Native
# par_for reads a `{` after the header as the loop body, like
# `for name ( word ... ) { list }` and `repeat n { list }`.
# Corpus site: none (split from #212 during implementation).
select o in a b c; { print $o; break }
select o in a b c; { print $o } && print x
select o in a b c; { print $o } | cat
select o in a b c; { break }; print after
select o { break }
select o; { break }
select o in a b c
{ break }
select o in a b c;
{ break }
select o in a b c; {
  # comment inside block
  print -r -- "$o"
  break
}
select o in a b c; {
  print -r -- "}"
  # comment containing }
  break
}
select o in a b c; {
  () { : }
  select o2 in x; { break }
  break
}
f() { select o in a; { break } }
while true; do select o in a; { break }; break; done
select o in '{' "}"; { break }
select o in a; { }
select o in a b c; break
select o in a b c; do break; done
