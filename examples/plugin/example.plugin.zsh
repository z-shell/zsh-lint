() {
  builtin emulate -L zsh

  local -r source_path=${1:a}
  local -r plugin_dir=${source_path:h}

  fpath+=( "${plugin_dir}/functions" "${plugin_dir}/completions" )
} "${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
