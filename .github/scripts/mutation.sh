#!/usr/bin/env bash
# Mutation-test changed Go source lines against a base (#425, #544).
#
# Dialect: Bash 4.0 or later. Usage, from anywhere inside the repository:
#
#     bash .github/scripts/mutation.sh [base-ref]   # default: origin/main
#
# Options precede the base: --candidate REV replays a committed candidate,
# --spec FILE extends generated mutants with JSON file, line, old, new entries,
# and --spec-only tests just those entries without repeating the generated set.
# --sample N selects up to N generated mutants randomly with --seed S (default
# 1); explicit --spec entries are always included. Sampling defaults to off and
# cannot combine with --spec-only or --max-mutants. --max-mutants N keeps the
# first N mutants for a diagnostic run. Incomplete evidence exits 3.
# --workers N uses 1 to 3 isolated snapshots (default 1). Each worker calibrates
# its timeout with a clean baseline at the requested concurrent load.
# --failfast=false disables the default go test -failfast for timing comparisons.
#
# The Go standard-library runner mutates comparisons, bounds, boolean guards
# and increments without requiring coverage blocks, including case guards and
# the parser fork. Fork mutants run their own package, then ./syntax/ (once),
# then the root parse/survey tests. LIVED requires every selected suite to pass.
# It tests an isolated snapshot, one mutant per build, never editing sources
# in place. Every build/test gets a process group that is killed on timeout
# and on normal completion so a descendant cannot outlive the run.
# MUTATION_TIMEOUT caps a mutant in seconds; otherwise its timeout is at least
# 30s and MUTATION_TIMEOUT_COEFFICIENT (default 30) times its baseline duration.
# Baselines run go test -vet=all; mutants run with -vet=off so vet diagnostics
# cannot decide them. Compile failures remain INVALID, not KILLED.
# Exit 0: all requested mutants killed (or no eligible changed Go source);
# a spec-only pass covers explicit entries only, never the generated campaign.
# 1: lived, even with unresolved results; 2: setup/baseline failure;
# 3: inconclusive (any timeout, invalid/unsupported, sampled or truncated run).
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
