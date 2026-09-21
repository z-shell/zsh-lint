#!/usr/bin/env zsh
# Regression fixture for #337. Each statement is valid Zsh; the defect only
# appeared when an alternate-form `if`, a `while` with a non-brace body, and
# a `try`/`always` block met in one file, so the combinations matter here
# more than the individual lines.

if (( 1 )) { x=1 }
while (( i < 3 )) (( i++ ))
{ true } always { true }

until (( done_flag )) (( done_flag=1 ))
{ : } always { : }

while (( a )) print loop
{ print body } always { print cleanup }

if [[ -n $HOME ]] { y=2 }
while [[ -n $x ]] unset x
{ true } always { true }

while (( n )) { print braced }
{ true } always { true }
