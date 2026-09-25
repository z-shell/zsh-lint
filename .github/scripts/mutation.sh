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
# gremlins derives each mutant's timeout from the baseline test time. With a
# fast suite its default is shorter than a mutant's build, so every mutant
# times out and none can live (#463). The coefficient below is set explicitly
# (MUTATION_TIMEOUT_COEFFICIENT overrides it), and a run in which timeouts
# outnumber the mutants that were killed or lived is reported as
# inconclusive and exits 3 rather than passing.
#
# Expect minutes, not seconds: every mutant reruns the tests of its package.

set -euo pipefail

gremlins_version=v0.6.0
base=${1:-origin/main}
timeout_coefficient=${MUTATION_TIMEOUT_COEFFICIENT:-30}

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
  --diff "$base" --output-statuses lc --timeout-coefficient "$timeout_coefficient" >"$report" 2>&1 || status=$?

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

# The summary lines read `Killed: K, Lived: L, Not covered: N` and
# `Timed out: T, ...`.
killed=$(sed -nE 's/^Killed: ([0-9]+),.*/\1/p' "$report")
timed_out=$(sed -nE 's/^Timed out: ([0-9]+),.*/\1/p' "$report")
killed=${killed:-0}
timed_out=${timed_out:-0}

echo "mutation: against $base, $lived lived, $uncovered on uncovered lines"
if ((lived > 0)); then
  exit 1
fi
if ((timed_out > killed + lived)); then
  echo "mutation: inconclusive: $timed_out mutants timed out, $killed killed; raise MUTATION_TIMEOUT_COEFFICIENT (now $timeout_coefficient)" >&2
  exit 3
fi
