#!/usr/bin/env zsh
# Fixture for #346.
# zshmisc spells a function body as a `list`, and a brace group is only one way
# to write one. Both definition spellings accept any statement as the body.
#
# `a () ; print z` does not run `print z`: it defines `a` with that body.

emulate -L zsh

paren_plain () ; print "paren-plain"

paren_if () ; if true; then print "paren-if"; fi

paren_for () ; for i in 1; do print "paren-for"; done

paren_case () ; case x in x) print "paren-case" ;; esac

paren_subshell () ; ( print "paren-subshell" )

paren_multi_a paren_multi_b () ; print "paren-multi"

function kw_plain; print "kw-plain"

function kw_while; while false; do :; done

function kw_subshell; ( print "kw-subshell" )

# The brace body keeps working in both spellings.
brace_paren () ; { print "brace-paren" }

function brace_kw; { print "brace-kw" }

paren_plain
paren_if
paren_for
paren_case
paren_subshell
paren_multi_a
paren_multi_b
kw_plain
kw_while
kw_subshell
brace_paren
brace_kw

print done-346
