#!/usr/bin/env bash
# Mutation-test changed Go source lines against a base (#425, #544).
#
# Dialect: Bash 4.0 or later. Usage, from anywhere inside the repository:
#
#     bash .github/scripts/mutation.sh [base-ref]   # default: origin/main
#
# Options precede the base: --candidate REV replays a committed candidate,
# --spec FILE extends generated mutants with JSON file, line, old, new entries,
# and --max-mutants N caps a diagnostic run (incomplete evidence exits 3).
#
# The Go standard-library runner mutates comparisons, bounds, boolean guards
# and increments without requiring coverage blocks, including case guards and
# the parser fork. Fork mutants run ./syntax/ and the root parse/survey tests.
# It tests an isolated snapshot, one mutant per build, never editing sources
# in place. Every build/test gets a process group that is killed on timeout
# and on normal completion so a descendant cannot outlive the run.
# MUTATION_TIMEOUT caps a mutant in seconds; otherwise its timeout is at least
# 30s and MUTATION_TIMEOUT_COEFFICIENT (default 30) times its baseline duration.
# Exit 0: all caught; 1: lived; 2: setup/baseline failure; 3: inconclusive
# (timeouts outnumber decided mutants, invalid/unsupported or truncated run).
#
# Expect minutes, not seconds: every mutant reruns the tests of its package.

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
binary=$(mktemp) || exit 2
trap 'rm -f "$binary"' EXIT
if ! go build -o "$binary" "$script_dir/mutation.go"; then
  echo 'mutation: runner build failed' >&2
  exit 2
fi
"$binary" "$@"
