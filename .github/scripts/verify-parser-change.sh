#!/usr/bin/env bash
# Verify a parser change against a base in one run (#560).
#
# Dialect: Bash 4.4 or later, where an empty array expands under set -u.
# Usage, from anywhere inside the repository:
#
#     bash .github/scripts/verify-parser-change.sh [options] [base-ref] [file.zsh ...]
#
#     --bodies FILE     probe these bodies (variants split by `---` lines) in
#                       every zsh-lint-probe context and compare the grid
#     --skip-mutation   leave out the mutation run (minutes, not seconds)
#
# base-ref defaults to origin/main. The files, typically the ones the change
# is about, join the corpus fixtures in the comparison and get a parse-count
# trace on both builds.
#
# Runs the checks of step 5 of .github/skills/parser-gap-fix/SKILL.md in
# order: build, vet and tests; the parser fork's tests; golangci-lint with the
# toolchain go.mod names; a base build exported with git archive; the
# -compare -native verdict comparison on the corpus fixtures and the given
# files; the probe grid; -trace-parses on both builds; and mutation.sh.
# Each check's output goes to its own log; the run prints one line per check,
# the tail of any failing log, and a Markdown table for the pull request.
#
# Exit status: 0 when every check passed or was skipped on request, 1 when a
# check failed, 3 when none failed but the mutation run was inconclusive, and
# 2 when the run could not start (unknown base, missing zsh or
# golangci-lint).

set -uo pipefail
export LC_ALL=C

bodies=
mutation=1
while (($# > 0)); do
  case $1 in
  --bodies)
    [[ $# -ge 2 ]] || {
      echo "verify: --bodies needs a file" >&2
      exit 2
    }
    bodies=$2
    shift 2
    ;;
  --skip-mutation)
    mutation=0
    shift
    ;;
  --)
    shift
    break
    ;;
  -*)
    echo "verify: unknown option: $1" >&2
    exit 2
    ;;
  *) break ;;
  esac
done
base=origin/main
if (($# > 0)) && [[ $1 != *.zsh ]]; then
  base=$1
  shift
fi
files=()
for file in "$@"; do
  [[ -r $file ]] || {
    echo "verify: cannot read $file" >&2
    exit 2
  }
  files+=("$file")
done
if [[ -n $bodies ]]; then
  [[ -r $bodies ]] || {
    echo "verify: cannot read $bodies" >&2
    exit 2
  }
  bodies=$(realpath "$bodies")
fi

root=$(git rev-parse --show-toplevel) || exit 2
cd "$root" || exit 2
# The checks run from the root: a file inside the repository is named
# relative to it, and one outside by its absolute path.
for i in "${!files[@]}"; do
  file=${files[i]}
  [[ $file == /* ]] || file=$OLDPWD/$file
  file=$(realpath "$file")
  files[i]=${file#"$root"/}
done
git rev-parse --verify --quiet "$base^{commit}" >/dev/null || {
  echo "verify: unknown base revision: $base" >&2
  exit 2
}
command -v zsh >/dev/null 2>&1 || {
  echo "verify: zsh is not on PATH; run bash .github/scripts/agent-setup.sh" >&2
  exit 2
}
# agent-setup.sh installs golangci-lint into GOBIN, which need not be on PATH.
lint=$(command -v golangci-lint) || {
  gobin=$(go env GOBIN)
  [[ -n $gobin ]] || gobin="$(go env GOPATH)/bin"
  lint=$gobin/golangci-lint
}
[[ -x $lint ]] || {
  echo "verify: golangci-lint not found; run bash .github/scripts/agent-setup.sh" >&2
  exit 2
}

work=$(mktemp -d)
logs=$work/logs
mkdir -p "$logs" "$work/base" "$work/bin"
trap 'rm -rf "$work/base" "$work/bin" "$work/grid"' EXIT

names=()
results=()
failed=0
inconclusive=0

# record NAME RESULT: RESULT is pass, FAIL, skipped or inconclusive.
record() {
  names+=("$1")
  results+=("$2")
  printf 'verify: %-18s %s\n' "$1" "$2"
  if [[ $2 == FAIL ]]; then
    failed=1
    echo "---- tail of $logs/$1.log ----"
    tail -n 30 "$logs/$1.log"
    echo "----"
  fi
}

# check NAME COMMAND...: runs COMMAND with its output in NAME's log.
check() {
  local name=$1
  shift
  if "$@" >"$logs/$name.log" 2>&1; then
    record "$name" pass
  else
    record "$name" FAIL
  fi
}

echo "verify: against $base; logs in $logs"

check test bash -c 'go build ./... && go vet ./... && go test ./...'
check fork-test bash -c 'cd third_party/mvdan-sh && go test ./syntax/ ./pattern/ ./expand/'
check lint env GOTOOLCHAIN="go$(go list -m -f '{{.GoVersion}}')" "$lint" run ./...

# The base is an export rather than a second worktree, so nothing is left to
# clean up in the repository.
if git archive "$base" 2>"$logs/build.log" | tar -x -C "$work/base" 2>>"$logs/build.log" &&
  (cd "$work/base" && go build -buildvcs=false -o "$work/bin/survey-base" ./cmd/zsh-lint-survey) >>"$logs/build.log" 2>&1 &&
  go build -o "$work/bin/survey-head" ./cmd/zsh-lint-survey >>"$logs/build.log" 2>&1 &&
  go build -o "$work/bin/probe" ./cmd/zsh-lint-probe >>"$logs/build.log" 2>&1; then
  record build pass
  check compare "$work/bin/survey-head" -compare "$work/bin/survey-base" -native \
    internal/survey/testdata/corpus/*.zsh "${files[@]}"
  if [[ -n $bodies ]]; then
    if "$work/bin/probe" -bodies "$bodies" -out "$work/grid" >"$logs/probe.log" 2>&1; then
      check probe "$work/bin/survey-head" -compare "$work/bin/survey-base" -native -known "$work/grid"/*.zsh
    else
      record probe FAIL
    fi
  else
    record probe skipped
  fi
  if ((${#files[@]} > 0)); then
    # A file with a parser gap exits non-zero; the trace on stderr is the
    # result, so only a missing trace fails the check.
    {
      echo "## base"
      { "$work/bin/survey-base" -trace-parses "${files[@]}" >/dev/null; } 2>&1
      echo "## head"
      { "$work/bin/survey-head" -trace-parses "${files[@]}" >/dev/null; } 2>&1
    } >"$logs/trace-parses.log"
    if grep -qv '^## ' "$logs/trace-parses.log"; then
      record trace-parses pass
    else
      record trace-parses FAIL
    fi
  else
    record trace-parses skipped
  fi
else
  record build FAIL
  record compare skipped
  record probe skipped
  record trace-parses skipped
fi

if ((mutation)); then
  status=0
  bash .github/scripts/mutation.sh "$base" >"$logs/mutation.log" 2>&1 || status=$?
  case $status in
  0) record mutation pass ;;
  3)
    record mutation inconclusive
    inconclusive=1
    ;;
  *) record mutation FAIL ;;
  esac
  # gremlins mutates only the root module, so a fork change reads as clean
  # without proving anything; hand-mutate it.
  if git diff --name-only "$base" -- third_party/mvdan-sh | grep -q .; then
    echo "verify: mutation.sh does not mutate third_party/mvdan-sh; hand-mutate the fork change"
  fi
else
  record mutation skipped
fi

echo
echo "| Check | Result |"
echo "| --- | --- |"
for i in "${!names[@]}"; do
  echo "| ${names[i]} | ${results[i]} |"
done
for name in compare probe trace-parses; do
  [[ -s $logs/$name.log ]] || continue
  echo
  echo "$name:"
  sed "s|$work/grid/|grid/|" "$logs/$name.log"
done
if [[ -s $logs/mutation.log ]]; then
  echo
  echo "mutation:"
  grep -E '^ *(LIVED|NOT COVERED) |^mutation: ' "$logs/mutation.log"
fi

if ((failed)); then
  exit 1
fi
if ((inconclusive)); then
  exit 3
fi
exit 0
