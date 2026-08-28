() {
  builtin emulate -L zsh
  local source_path=$1

  fpath+=( "${source_path:h}/functions" "${source_path:h}/completions" )
} "${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
