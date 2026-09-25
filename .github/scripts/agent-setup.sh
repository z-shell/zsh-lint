#!/usr/bin/env bash
# Prepare an agent or contributor session for zsh-lint work (#416).
#
# Dialect: Bash 4.0 or later. Run from anywhere inside the repository:
#
#     bash .github/scripts/agent-setup.sh
#
# Idempotent: each step checks first and changes nothing when the tool is
# already present at the expected version. It installs:
#
# - zsh, the native oracle for parser work, through apt-get when it is missing
#   (with sudo when not root), plus zsh-doc for the manual when available;
# - golangci-lint at the version Go CI runs, built with the Go release go.mod
#   names (Go CI's toolchain). golangci-lint loads the standard library of the
#   `go` it runs with, and v2.12.2 cannot analyze a newer release's library,
#   so on a machine with a newer Go it must run as
#   `GOTOOLCHAIN=go<version> golangci-lint run ./...`; the script prints the
#   exact command;
# and it downloads the module dependencies and builds the module once.

set -euo pipefail

golangci_version=v2.12.2

root=$(git rev-parse --show-toplevel)
cd "$root"

say() { printf 'agent-setup: %s\n' "$*"; }

as_root() {
  if ((EUID == 0)); then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo -n "$@"
  else
    return 1
  fi
}

if command -v zsh >/dev/null 2>&1; then
  say "zsh present: $(zsh --version)"
elif command -v apt-get >/dev/null 2>&1; then
  say "installing zsh"
  if as_root apt-get update -qq && as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq zsh; then
    as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq zsh-doc >/dev/null 2>&1 ||
      say "zsh-doc unavailable; read the manual at https://zsh.sourceforge.io/Doc/"
    say "zsh installed: $(zsh --version)"
  else
    say "could not install zsh (no root or sudo); native-oracle tests will skip"
  fi
else
  say "zsh missing and no apt-get; install zsh by hand, native-oracle tests will skip"
fi

# The release go.mod names, as a toolchain name (1.26.0 -> go1.26.0).
lint_toolchain="go$(go list -m -f '{{.GoVersion}}')"
lint_minor=${lint_toolchain%.*}

gobin=$(go env GOBIN)
[[ -n $gobin ]] || gobin="$(go env GOPATH)/bin"
lint="$gobin/golangci-lint"
if [[ -x $lint ]] && "$lint" version 2>/dev/null | grep -q "version ${golangci_version#v} built with ${lint_minor}[.-]"; then
  say "golangci-lint present: $("$lint" version | head -n 1)"
else
  say "installing golangci-lint $golangci_version with $lint_toolchain"
  GOTOOLCHAIN=$lint_toolchain go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$golangci_version"
  say "golangci-lint installed: $("$lint" version | head -n 1)"
fi
case :$PATH: in
*":$gobin:"*) ;;
*) say "add $gobin to PATH to run golangci-lint by name" ;;
esac
local_go=$(GOTOOLCHAIN=local go env GOVERSION)
if [[ $local_go == "$lint_minor" || $local_go == "$lint_minor".* || $local_go == "$lint_minor"-* ]]; then
  say "lint with: golangci-lint run ./..."
else
  say "local Go is $local_go; lint with: GOTOOLCHAIN=$lint_toolchain golangci-lint run ./..."
fi

go mod download
go build ./...
say "module downloaded and built with $(go env GOVERSION)"
