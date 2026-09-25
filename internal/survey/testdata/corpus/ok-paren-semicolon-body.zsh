#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Fixture for #307.
# zshmisc spells a function definition as `word ... () [ term ] { list }`, so
# the separator before the body brace is optional in the `()` spelling exactly
# as it is after the `function` keyword (#213). The `()` head may carry more
# than one name, which composes with the multi-name adapter (#304).

emulate -L zsh

single () ; { print single }

two_a two_b () ; { print two }

three_a three_b three_c () ; { print three }

newline_after_separator () ;
{ print newline }

with_statements () ; { print one; print two }

# The keyword spelling, unchanged by this adapter.
function keyword_form; { print keyword }

function keyword_parens () ; { print keyword-parens }

single
two_a
three_a
newline_after_separator
with_statements
keyword_form
keyword_parens
