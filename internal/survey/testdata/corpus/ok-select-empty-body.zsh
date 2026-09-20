# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #302: a select name [in word ...] header followed by an empty body is
# native-valid Zsh (par_for reads one sublist after the header and par_sublist
# accepts an empty one), so a header at end of file, or directly before a closer
# such as } of a function body or ) of a subshell, is valid.
# Corpus site: none (split from #212 during implementation).
f() { select o in a b c; }
( select o in a b c; )
g() { select o; }
h() {
  select o in a b c;

  # nothing here
}
i() { select o in a b c
}
j() { select o in a; select p in b; }
k() { select o in a b c; }; print after
while true; do select o in a; done
l() { select o in '}' "x"; }
select o in a b c; do; done
select o in a b c; break
select o in a b c;
if select o in a; then :; fi
