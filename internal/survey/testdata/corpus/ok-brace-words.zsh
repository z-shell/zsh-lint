# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #314: a bare `}` is never a word, but a quoted or escaped `}`, a
# bare `{`, a `}` on a here-document line and a `}` inside an expansion are.
# The front end rejects the bare word (invalid-314-*.txt) and must keep
# reading every other spelling as Zsh does.
for x in '}' "}" \} {; do print -r -- "$x"; done
select o in '}' {; break
x=( a { '}' "}" \} )
case '}' in ('}') print -r -- closer ;; esac
print -r -- {
print -r -- "${x[1]}" ${x:-'}'}
cat <<EOF
}
{
EOF
f() { print -r -- '}' }
{ print -r -- \} }
