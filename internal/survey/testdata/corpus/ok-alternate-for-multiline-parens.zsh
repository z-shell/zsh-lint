# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Issue #263: the parenthesized word list of an alternate `for name ( words )
# { ... }` loop may span several lines; each bare newline separates words the
# same way a space does.
# Preserved as permanent regression coverage.

typeset -A ZI
ZI[HOME_DIR]=/tmp/zi ZI[CACHE_DIR]=/tmp/zi/cache ZI[PLUGINS_DIR]=/tmp/zi/plugins

# The corpus shape: list opens after `(`, wraps over several lines, and the
# closing `)` stands on its own line.
local required
for required (
  ${ZI[HOME_DIR]} ${ZI[CACHE_DIR]}
  ${ZI[PLUGINS_DIR]} ${ZI[PLUGINS_DIR]}/_local---zi
) {
  [[ -d $required ]] || print -r -- "missing: $required"
}

# Wrap before the closing paren only.
for item ( one two
  three ) {
  print -r -- "$item"
}

# Blank lines inside the list.
for item (
  one

  two
) {
  print -r -- "$item"
}

# A backslash continuation and a quoted newline are part of their words and
# stay as they are.
for item ( one \
  two "three
four" ) {
  print -r -- "$item"
}

# A newline inside a command substitution belongs to the substitution.
for item ($(print one
print two) three) {
  print -r -- "$item"
}

# A quoted or escaped `)` is part of its word and does not close the list.
for item ( "one)
two" three\) ) {
  print -r -- "$item"
}
