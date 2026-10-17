#!/usr/bin/env bash
# User-level fork confirmation shells. Bash 4.0+, Linux/macOS with a C compiler.
# Sourced by agent-setup.sh so its PATH also covers that script's checks.

shell_prefix=${ZSH_LINT_SHELL_PREFIX:-${XDG_CACHE_HOME:-$HOME/.cache}/zsh-lint/fork-shells}
mkdir -p "$shell_prefix"
shell_prefix=$(cd "$shell_prefix" && pwd -P)
mkdir -p "$shell_prefix/bin" "$shell_prefix/downloads" "$shell_prefix/build"
export PATH="$shell_prefix/bin:$PATH"

shell_download() {
  local name=$1 url=$2 checksum=$3
  local archive=$shell_prefix/downloads/$name
  if [[ ! -f $archive ]]; then
    curl --fail --show-error --silent --location --proto '=https' --proto-redir '=https' \
      --retry 3 --output "$archive.part" "$url"
    mv "$archive.part" "$archive"
  fi
  printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check --status || {
    printf 'agent-setup: checksum mismatch: %s\n' "$archive" >&2
    return 1
  }
  # Build trees remain in the user-owned cache for diagnosis; never remove
  # someone else's temp directories or package-manager files.
  tar -xf "$archive" -C "$shell_prefix/build"
}

shell_has_dash() {
  local text
  # shellcheck disable=SC2016 # The compatibility probe is expanded by dash.
  text=$(dash -c 'echo ${#<}' 2>&1) || :
  [[ $text == *'Bad subst'* ]]
}
shell_has_mksh() {
  local text
  # shellcheck disable=SC2016 # Read the child shell's version, not Bash's.
  text=$(mksh -c 'echo "$KSH_VERSION"' 2>/dev/null) || return 1
  [[ $text == *' R59 '* ]]
}
shell_has_bash53() {
  local text
  text=$(bash --version) || return 1
  [[ $text == *'version 5.3.'* ]]
}

if ! shell_has_dash; then
  # Release archive checksum also recorded by Void's dash package.
  shell_download dash-0.5.12.tar.gz \
    https://deb.debian.org/debian/pool/main/d/dash/dash_0.5.12.orig.tar.gz \
    6a474ac46e8b0b32916c4c60df694c82058d3297d8b385b74508030ca4a8f28a
  (
    cd "$shell_prefix/build/dash-0.5.12" || exit 1
    ./configure --prefix="$shell_prefix" --bindir="$shell_prefix/bin" --without-libedit
    make -j2
    make install
  )
fi
if ! shell_has_mksh; then
  # Upstream R59c archive, checksum also recorded by Void's mksh package.
  shell_download mksh-R59c.tgz \
    https://deb.debian.org/debian/pool/main/m/mksh/mksh_59c.orig.tar.gz \
    77ae1665a337f1c48c61d6b961db3e52119b38e58884d1c89684af31f87bc506
  (
    cd "$shell_prefix/build/mksh" || exit 1
    sh ./Build.sh -r
    cp mksh "$shell_prefix/bin/mksh"
  )
fi
if ! shell_has_bash53; then
  # Same GNU release and checksum as the Go CI confirmation step.
  shell_download bash-5.3.tar.gz \
    https://ftp.gnu.org/gnu/bash/bash-5.3.tar.gz \
    0d5cd86965f869a26cf64f4b71be7b96f90a3ba8b3d74e27e8e9d9d5550f31ba
  (
    cd "$shell_prefix/build/bash-5.3" || exit 1
    ./configure --prefix="$shell_prefix" --disable-readline
    make -j2
    make install
  )
fi
if ! shell_has_dash || ! shell_has_mksh || ! shell_has_bash53; then
  printf '%s\n' 'agent-setup: fork confirmation-shell checks failed' >&2
  return 1
fi
# shellcheck disable=SC2016 # Print an activation command for the caller.
printf 'agent-setup: fork shells ready; activate with: export PATH=%q:$PATH\n' "$shell_prefix/bin"
