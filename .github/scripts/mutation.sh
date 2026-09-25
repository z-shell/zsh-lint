#!/usr/bin/env bash
# Mutation-test the lines the working tree changes against a base (#425).
#
# Dialect: Bash 4.0 or later. Usage, from anywhere inside the repository:
#
#     bash .github/scripts/mutation.sh [base-ref]   # default: origin/main
#
# Runs gremlins (pinned below) on every mutant of a changed line and runs the
# tests against each. A mutant the tests kill, or one that makes them hang,
# is detected: a scanner-loop mutant such as `i++` to `i--` hangs the suite,
# so gremlins reports it as TIMED OUT, and that counts as caught. A mutant
# that LIVED is a test gap; the script lists them and exits 1. Changed lines
# no test covers are listed too, as NOT COVERED, without failing the run.
#
# Expect minutes, not seconds: every mutant reruns the tests of its package.

set -euo pipefail

gremlins_version=v0.6.0
base=${1:-origin/main}

root=$(git rev-parse --show-toplevel)
cd "$root"
git rev-parse --verify --quiet "$base^{commit}" >/dev/null || {
  echo "mutation: unknown base revision: $base" >&2
  exit 2
}

report=$(mktemp)
trap 'rm -f "$report"' EXIT

status=0
go run "github.com/go-gremlins/gremlins/cmd/gremlins@$gremlins_version" unleash \
  --diff "$base" --output-statuses lc >"$report" 2>&1 || status=$?

lived=$(grep -cE '^ *LIVED ' "$report" || true)
uncovered=$(grep -cE '^ *NOT COVERED ' "$report" || true)

grep -E '^ *(LIVED|NOT COVERED) ' "$report" || true
grep -E '^(Killed|Timed out|Test efficacy|Mutator coverage):' "$report" || true

if ((status != 0)); then
  echo "mutation: gremlins exited $status" >&2
  tail -n 20 "$report" >&2
  exit 2
fi
if ! grep -q '^Killed:' "$report"; then
  echo "mutation: gremlins printed no summary" >&2
  tail -n 20 "$report" >&2
  exit 2
fi

echo "mutation: against $base, $lived lived, $uncovered on uncovered lines"
if ((lived > 0)); then
  exit 1
fi
