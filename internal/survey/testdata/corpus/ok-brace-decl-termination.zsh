# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
# Issue #231 and #259: a declaration builtin or `let` may be the last command
# before `}` with no separator. The parser reads the clause up to a stop token
# and a `}` word is not one, so zsh-lint closes the block with a masked
# separator and clears it from the tree. `nameref` is here because the parser
# dispatches it as a declaration word, not because Zsh has such a builtin.
{ typeset -g COLS="$(tput cols)" } 2>/dev/null
{ local C=1 }
{ export C=1 }
{ readonly R=1 }
{ declare D=1 }
{ nameref N=C }
{ let L=1 }
{ typeset -g C=1 } && { typeset -g D=2 }
{ local x=1 }; { local y=2 }
{ local x=1 } always { local y=2 }
{ { local x=1 } }
( { local x=1 } )
v=$({ local y=1 })
v=`{ local y=1 }`
case x in a) { local y=1 } ;; esac
{ local x=1 } | cat
{ local x=1 }>/dev/null
{ local x=1 }&
{ local x=1 } # comment
{ typeset x="}" y='}' }
{ local -a a b }
{ local x=(1 2) }
{ local x=${y} }
{ typeset -g x=$(a | b) }
{ local x=$(f; g) y=`h` }
{ local x=`a | b` }
{ print a; local x=1 }
{ local x=1; typeset -g y=2 }
{ ! local x=1 }
{ time local x=1 }
{ local x=1 \
}
f() { local x=1 }
g() { let i++ }
() { local x=1 }
h() {
  print a
  local x=1 }
wait
