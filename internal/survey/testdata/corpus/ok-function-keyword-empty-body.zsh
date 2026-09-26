#!/usr/bin/env zsh
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
# Fixture for #479: a function keyword definition whose body is missing or is
# itself a bodyless or keyword definition defines every name with an empty body.

function a
function a b c
function f print hi
function a;
function a b;
function a b c;

function
function ;
function a ()
function a () ;
{ function a; }
{ function a }
x=$(function a)
(function a)
if true; then function a; fi
a() { function b; }
f () ; function a
function a; function b
function a { }
function b
function a; function b; print z
a
b
