# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
# Issue #298: a declaration builtin before a bare `}` inside a loop, `if` or
# `case` body, with a separator after the brace. The clause swallows the `}`
# and stops at the `;`, so the parser meets `done`, `fi` or `esac` inside the
# open block and reports that keyword instead of the unclosed brace.
# Preserved as permanent regression coverage.
while true; do { local x }; done
while true; do { local x }
done
if true; then { local x }; fi
f() { while true; do { local x }; done }
until false; do { typeset -g y=1 }; done
for i in 1; do { export z=2 }; done
select i in 1; do { local x }; done
repeat 2 do { local x }; done
if true; then :; elif true; then { local x }; fi
if true; then :; else { local x }; fi
if true; then { local x }; elif true; then :; fi
if true; then { local x }; else :; fi
case x in (x) { local x }; esac
case x in (x) { local x }
esac
while { local x }; do :; done
if { local x }; then :; fi
while true; do
  { local x }
done
while true; do { local x } # comment
done
while true; do { local x }; { local y }; done
while true; do { local x } | cat; done
while true; do { local x } && :; done
g() { if true; then { local x }; fi }
while true; do { local x } done
if true; then { local x } fi
