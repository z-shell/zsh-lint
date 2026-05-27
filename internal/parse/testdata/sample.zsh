#!/usr/bin/env zsh
# Minimal fixture for the parser-evaluation harness.
local name="world"
print -r -- "hello, ${name}"
for i in 1 2 3; do
  print -r -- "count: ${i}"
done
