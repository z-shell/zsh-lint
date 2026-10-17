#!/usr/bin/env bash
# Verify a parser change against a base in one run (#560, #545).
#
# Dialect: Bash 4.4 or later, where an empty array expands under set -u.
# Usage, from anywhere inside the repository:
#
#     bash .github/scripts/verify-parser-change.sh [options] [base-ref] [file.zsh ...]
#     bash .github/scripts/verify-parser-change.sh --diagnostics [diagnostic options]
#
# --diagnostics must be first; it compares analyzer JSON without running parser
# grids, tests or mutation. See docs/project/diagnostic-diff.md for its options.
#
#     --bodies FILE     probe these bodies (variants split by `---` lines) in
#                       every zsh-lint-probe context and compare the grid
#     --rows FILE       probe these rows, one complete source per line, for a
#                       grid the contexts cannot express (repeatable)
#     --root DIR        also compare every regular file under DIR (repeatable)
#     --list FILE       also compare the files FILE names, one per line or
#                       NUL-separated (repeatable)
#     --regression-corpus DIR
#                       run the Corpus Gate's regression-corpus.sh over
#                       DIR/<path>/<root> as regression-corpus.txt lists them
#     --candidate REV   judge the build of REV instead of the working tree,
#                       to re-run a merged change's verification; the Go
#                       checks and mutation then do not run
#     --skip-mutation   leave out the mutation run (minutes, not seconds)
#     --full-mutation   run the uncapped campaign instead of the default
#                       30-mutant sample with seed 1 (use at batch boundaries)
#     --output DIR      retain logs and summary.md in a new directory outside
#                       the checkout; its parent must already exist
#     --check-base      check the base and the candidate, print both ids and
#                       stop, without building anything
#
# base-ref defaults to origin/main, and then the run refuses to start unless
# the local origin/main is origin's main, checked with `git ls-remote`, which
# fetches nothing; a named base-ref skips that check. Either way the
# candidate must contain the base, or the base's own fixes would read as
# regressions (#545). Both commit ids are printed. The files, typically the
# ones the change is about, join the corpus fixtures in the comparison and
# get a parse-count trace on both builds.
#
# Runs the checks of step 5 of .github/skills/parser-gap-fix/SKILL.md in
# order: build, vet and tests; the parser fork's tests; golangci-lint with the
# toolchain go.mod names; a base build exported with git archive; the
# -compare -native verdict comparison on the corpus fixtures and the given
# files, roots and lists; the regression corpus; the probe grids;
# -trace-parses on both builds; and mutation.sh --sample 30 --seed 1.
# Sampled survivors and incomplete mutation evidence are inconclusive, not
# functional failures. --full-mutation retains the runner's failure verdict.
#
# A probe grid is also judged with -runtime: a row that `zsh -f -n` accepts
# and the change now rejects is RUNTIME-REJECTED rather than REGRESSED when
# Zsh rejects it as it runs, and that does not fail the check. -runtime runs
# the rows, which is why no other comparison uses it. Each grid writes its
# changed rows as a survey-record table, <check>.md, next to the logs.
#
# Each check's output goes to its own log; the run prints one line per check,
# the tail of any failing log, and a Markdown table for the pull request.
#
# Exit status: 0 when every check passed or was skipped on request, 1 when a
# check failed, 3 when none failed but the mutation run was inconclusive, and
# 2 when the run could not start (unknown or stale base, a candidate without
# the base, an unreadable argument, missing zsh or golangci-lint).

set -uo pipefail
export LC_ALL=C

if [[ ${1-} == --diagnostics ]]; then
  shift
  script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
  binary=$(mktemp) || exit 2
  trap 'rm -f "$binary"' EXIT
  if ! go build -o "$binary" "$script_dir/verify-diagnostics.go"; then
    echo 'verify-diagnostics: runner build failed' >&2
    exit 2
  fi
  "$binary" "$@"
  exit $?
fi

die() {
  echo "verify: $*" >&2
  exit 2
}
# need OPTION ARGC: fail when OPTION has no argument.
need() { (($2 >= 2)) || die "$1 needs an argument"; }

bodies=
candidate_ref=
regression_corpus=
mutation=1
full_mutation=0
output=
check_base=0
rows=()
roots=()
lists=()
while (($# > 0)); do
  case $1 in
  --bodies)
    need "$1" $#
    bodies=$2
    shift 2
    ;;
  --rows)
    need "$1" $#
    rows+=("$2")
    shift 2
    ;;
  --root)
    need "$1" $#
    roots+=("$2")
    shift 2
    ;;
  --list)
    need "$1" $#
    lists+=("$2")
    shift 2
    ;;
  --regression-corpus)
    need "$1" $#
    regression_corpus=$2
    shift 2
    ;;
  --candidate)
    need "$1" $#
    candidate_ref=$2
    shift 2
    ;;
  --skip-mutation)
    mutation=0
    shift
    ;;
  --full-mutation)
    full_mutation=1
    shift
    ;;
  --output)
    need "$1" $#
    output=$2
    shift 2
    ;;
  --check-base)
    check_base=1
    shift
    ;;
  --)
    shift
    break
    ;;
  -*) die "unknown option: $1" ;;
  *) break ;;
  esac
done
((mutation || !full_mutation)) || die "--full-mutation cannot combine with --skip-mutation"
base=origin/main
named_base=0
if (($# > 0)) && [[ $1 != *.zsh ]]; then
  base=$1
  named_base=1
  shift
fi
files=()
for file in "$@"; do
  [[ -r $file ]] || die "cannot read $file"
  files+=("$file")
done
# Every path argument is relative to the caller's directory.
if [[ -n $bodies ]]; then
  [[ -r $bodies ]] || die "cannot read $bodies"
  bodies=$(realpath "$bodies")
fi
for i in "${!rows[@]}"; do
  [[ -r ${rows[i]} ]] || die "cannot read ${rows[i]}"
  rows[i]=$(realpath "${rows[i]}")
done
for i in "${!roots[@]}"; do
  [[ -d ${roots[i]} ]] || die "not a directory: ${roots[i]}"
  roots[i]=$(realpath "${roots[i]}")
done
for i in "${!lists[@]}"; do
  [[ -r ${lists[i]} ]] || die "cannot read ${lists[i]}"
  lists[i]=$(realpath "${lists[i]}")
done
if [[ -n $regression_corpus ]]; then
  [[ -d $regression_corpus ]] || die "not a directory: $regression_corpus"
  regression_corpus=$(realpath "$regression_corpus")
fi
if [[ -n $output ]]; then
  parent=$(cd -- "$(dirname -- "$output")" && pwd -P) || die "output parent must exist"
  output=$parent/$(basename -- "$output")
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

# The base guard: a measurement once ran 37 commits behind origin/main
# without noticing (#545).
base_id=$(git rev-parse --verify --quiet "$base^{commit}") || die "unknown base revision: $base"
if ((!named_base)); then
  remote=$(git ls-remote --exit-code origin refs/heads/main 2>/dev/null | cut -f1) ||
    die "cannot read origin's main to check that origin/main is current; name the base revision"
  [[ $remote == "$base_id" ]] ||
    die "origin/main is ${base_id:0:12} but origin's main is ${remote:0:12}; run \`git fetch origin\` and rebase, or name the base revision"
fi
if [[ -n $candidate_ref ]]; then
  head_id=$(git rev-parse --verify --quiet "$candidate_ref^{commit}") || die "unknown candidate revision: $candidate_ref"
  head_what="the candidate"
  head_note="candidate $candidate_ref"
else
  head_id=$(git rev-parse HEAD)
  head_what=HEAD
  head_note=HEAD
  [[ -z $(git status --porcelain) ]] || head_note="HEAD plus uncommitted changes"
fi
git merge-base --is-ancestor "$base_id" "$head_id" ||
  die "$head_what ${head_id:0:12} does not contain the base ${base_id:0:12}; rebase onto it first, or its fixes will read as regressions"
echo "verify: base      $base_id ($base)"
echo "verify: candidate $head_id ($head_note)"
((!check_base)) || exit 0

command -v zsh >/dev/null 2>&1 || die "zsh is not on PATH; run bash .github/scripts/agent-setup.sh"
# agent-setup.sh installs golangci-lint into GOBIN, which need not be on PATH.
lint=$(command -v golangci-lint) || {
  gobin=$(go env GOBIN)
  [[ -n $gobin ]] || gobin="$(go env GOPATH)/bin"
  lint=$gobin/golangci-lint
}
if [[ -z $candidate_ref && ! -x $lint ]]; then
  die "golangci-lint not found; run bash .github/scripts/agent-setup.sh"
fi

work=$(mktemp -d)
logs=$work/logs
if [[ -n $output ]]; then
  [[ $output != "$root" && $output != "$root/"* ]] || die "output must be outside the checkout"
  [[ ! -e $output && ! -L $output ]] || die "output already exists: $output"
  mkdir -- "$output" || die "cannot create output directory"
  logs=$output
fi
mkdir -p "$logs" "$work/base" "$work/bin"
trap 'rm -rf "$work/base" "$work/candidate" "$work/bin" "$work/grids" "$work/regression"' EXIT

names=()
results=()
durations=()
failed=0
inconclusive=0

# Include untracked source bytes, not just HEAD or the tracked diff. Generated
# ignored runtime files and retained logs outside the checkout are excluded.
fingerprint() {
  {
    git rev-parse HEAD || return
    git diff --binary HEAD || return
    while IFS= read -r -d '' file; do
      printf '%s\0' "$file"
      git hash-object -- "$file" || return
    done < <(git ls-files --others --exclude-standard -z)
  } | git hash-object --stdin
}
source_id=$(fingerprint) || die "cannot fingerprint source"
{
  echo "base: $base_id"
  echo "candidate: $head_id ($head_note)"
  echo "verification source: $source_id"
  echo "mutation policy: sample 30, seed 1 (full=$full_mutation, enabled=$mutation)"
  zsh --version
  go version
  bash --version | head -n 1
  if [[ -z $candidate_ref ]]; then "$lint" version; fi
} >"$logs/identity.log"

# record NAME RESULT: RESULT is pass, FAIL, skipped or inconclusive.
record() {
  names+=("$1")
  results+=("$2")
  durations+=("${3:--}")
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
  local started=$SECONDS
  shift
  { printf '%q ' "$@"; echo; } >>"$logs/commands.log"
  if "$@" >"$logs/$name.log" 2>&1; then
    record "$name" pass "$((SECONDS - started))"
  else
    record "$name" FAIL "$((SECONDS - started))"
  fi
}

echo "verify: logs in $logs"

if [[ -z $candidate_ref ]]; then
  check test bash -c 'go build ./... && go vet -tags=integration ./... && go test ./...'
  check integration-test go test -tags=integration ./internal/workflowcontract/
  check fork-test bash -c 'cd third_party/mvdan-sh && go test ./syntax/ ./pattern/ ./expand/'
  check lint env GOTOOLCHAIN="go$(go list -m -f '{{.GoVersion}}')" "$lint" run ./...
else
  record test skipped
  record integration-test skipped
  record fork-test skipped
  record lint skipped
fi

# The base and a named candidate are exports rather than second worktrees,
# so nothing is left to clean up in the repository. Every comparison runs
# through this checkout's zsh-lint-survey with -candidate, so its classes are
# today's whichever two builds it compares.
candidate_dir=$root
build() {
  git archive "$base_id" | tar -x -C "$work/base" || return
  (cd "$work/base" && go build -buildvcs=false -o "$work/bin/survey-base" ./cmd/zsh-lint-survey) || return
  if [[ -n $candidate_ref ]]; then
    candidate_dir=$work/candidate
    mkdir -p "$candidate_dir" || return
    git archive "$head_id" | tar -x -C "$candidate_dir" || return
  fi
  (cd "$candidate_dir" && go build -buildvcs=false -o "$work/bin/survey-head" ./cmd/zsh-lint-survey) || return
  go build -o "$work/bin/survey" ./cmd/zsh-lint-survey || return
  go build -o "$work/bin/probe" ./cmd/zsh-lint-probe
}
build_started=$SECONDS
if build >"$logs/build.log" 2>&1; then
  record build pass "$((SECONDS - build_started))"
  compare=("$work/bin/survey" -compare "$work/bin/survey-base" -candidate "$work/bin/survey-head" -native)
  corpus=("$candidate_dir"/internal/survey/testdata/corpus/*.zsh)
  check compare "${compare[@]}" "${corpus[@]#"$root"/}" "${files[@]}"

  index=0
  for dir in "${roots[@]}"; do
    index=$((index + 1))
    mapfile -d '' found < <(find "$dir" -name .git -prune -o -type f -print0 | sort -z)
    if ((${#found[@]} == 0)); then
      echo "no files under $dir" >"$logs/root-$index.log"
      record "root-$index" FAIL
    else
      check "root-$index" "${compare[@]}" "${found[@]}"
    fi
  done
  index=0
  for list in "${lists[@]}"; do
    index=$((index + 1))
    if (($(tr -dc '\000' <"$list" | wc -c) > 0)); then
      mapfile -d '' found <"$list"
    else
      mapfile -t found < <(grep -v '^$' "$list")
    fi
    # A missing file fails to open in both builds and would read as
    # unchanged, so it fails the check instead.
    missing=()
    for file in "${found[@]}"; do
      [[ -f $file ]] || missing+=("$file")
    done
    if ((${#found[@]} == 0 || ${#missing[@]} > 0)); then
      {
        echo "$list: ${#missing[@]} of ${#found[@]} path(s) are not files"
        printf '%s\n' "${missing[@]:0:10}"
      } >"$logs/list-$index.log"
      record "list-$index" FAIL
    else
      check "list-$index" "${compare[@]}" "${found[@]}"
    fi
  done

  if [[ -n $regression_corpus ]]; then
    # The Corpus Gate's own step: from a directory holding zsh-lint/ and
    # corpus/, it runs $RUNNER_TEMP/survey-head against survey-base.
    mkdir -p "$work/regression"
    ln -s "$candidate_dir" "$work/regression/zsh-lint"
    ln -s "$regression_corpus" "$work/regression/corpus"
    # shellcheck disable=SC2016 # expanded by the inner bash
    check regression-corpus bash -c 'cd "$1" && RUNNER_TEMP=$2 GITHUB_STEP_SUMMARY=/dev/null bash zsh-lint/.github/scripts/regression-corpus.sh' \
      regression-corpus "$work/regression" "$work/bin"
    while read -r path _ revision _; do
      [[ -n $path && -e $regression_corpus/$path/.git ]] || continue
      actual=$(git -C "$regression_corpus/$path" rev-parse HEAD 2>/dev/null) || continue
      [[ $actual == "$revision" ]] ||
        echo "verify: $regression_corpus/$path is at ${actual:0:12}, regression-corpus.txt pins ${revision:0:12}"
    done <"$candidate_dir/docs/project/regression-corpus.txt"
  fi

  # Probe grids: the bodies file, then each rows file. One grid is named
  # probe, several probe-1, probe-2 and so on.
  grids=()
  [[ -z $bodies ]] || grids+=(-bodies "$bodies")
  for file in "${rows[@]}"; do grids+=(-rows "$file"); done
  count=$((${#grids[@]} / 2))
  if ((count == 0)); then
    record probe skipped
  fi
  for ((index = 1; index <= count; index++)); do
    name=probe
    ((count == 1)) || name=probe-$index
    grid=$work/grids/$index
    mkdir -p "$work/grids"
    if "$work/bin/probe" "${grids[2 * index - 2]}" "${grids[2 * index - 1]}" -out "$grid" >"$logs/$name.log" 2>&1; then
      check "$name" "${compare[@]}" -known -runtime -table "$logs/$name.md" "$grid"/*.zsh
    else
      record "$name" FAIL
    fi
  done

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
  record build FAIL "$((SECONDS - build_started))"
  record compare skipped
  record probe skipped
  record trace-parses skipped
fi

if ((mutation && !failed)) && [[ -z $candidate_ref ]]; then
  status=0
  mutation_options=(--sample 30 --seed 1)
  (( !full_mutation )) || mutation_options=()
  { printf '%q ' bash .github/scripts/mutation.sh "${mutation_options[@]}" "$base_id"; echo; } >>"$logs/commands.log"
  mutation_started=$SECONDS
  bash .github/scripts/mutation.sh "${mutation_options[@]}" "$base_id" >"$logs/mutation.log" 2>&1 || status=$?
  case $status in
  0) record mutation pass "$((SECONDS - mutation_started))" ;;
  1)
    if ((full_mutation)); then
      record mutation FAIL "$((SECONDS - mutation_started))"
    else
      record mutation inconclusive "$((SECONDS - mutation_started))"
      inconclusive=1
    fi
    ;;
  3)
    record mutation inconclusive "$((SECONDS - mutation_started))"
    inconclusive=1
    ;;
  *) record mutation FAIL "$((SECONDS - mutation_started))" ;;
  esac
else
  record mutation skipped
fi

if [[ $(fingerprint) != "$source_id" ]]; then
  echo "source changed during verification; rerun on the final candidate" >"$logs/source.log"
  record source FAIL
fi

report() {
cat "$logs/identity.log"
echo
echo "| Check | Result | Seconds |"
echo "| --- | --- | --- |"
for i in "${!names[@]}"; do
  echo "| ${names[i]} | ${results[i]} | ${durations[i]} |"
done
for i in "${!names[@]}"; do
  name=${names[i]}
  [[ ${results[i]} != skipped && -s $logs/$name.log ]] || continue
  case $name in
  probe | probe-*)
    # A grid's changed rows can run to thousands; its table file holds them.
    echo
    echo "$name:"
    tail -n 1 "$logs/$name.log"
    # The table has two header lines; anything more is a changed row.
    if (($(wc -l <"$logs/$name.md" 2>/dev/null || echo 0) > 2)); then
      echo "changed rows: $logs/$name.md"
    fi
    ;;
  compare | root-* | list-* | regression-corpus | trace-parses)
    echo
    echo "$name:"
    sed "s|$root/||g" "$logs/$name.log"
    ;;
  *) ;;
  esac
done
if [[ -s $logs/mutation.log ]]; then
  echo
  echo "mutation:"
  # Unstructured setup errors may have no summary lines; keep the real verdict.
  grep -E '^ *(KILLED|LIVED|TIMED OUT|INVALID|UNSUPPORTED) |^mutation: ' "$logs/mutation.log" || [[ $? == 1 ]]
fi
}
report | tee "$logs/summary.md" || die "cannot write verification summary"

if ((failed)); then
  exit 1
fi
if ((inconclusive)); then
  exit 3
fi
exit 0
