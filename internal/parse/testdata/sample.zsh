#!/usr/bin/env zsh
# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Minimal fixture for the parser-evaluation harness.
local name="world"
print -r -- "hello, ${name}"
for i in 1 2 3; do
  print -r -- "count: ${i}"
done
