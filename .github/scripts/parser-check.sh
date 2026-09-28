#!/usr/bin/env bash
# Run a parser fix's verification in one command (#545).
#
# Dialect: Bash 4.4 or later (mapfile -d, namerefs, empty arrays under set -u). Run from anywhere inside the
# repository, in the foreground:
#
#     bash .github/scripts/parser-check.sh --out DIR [options]
#
# Steps, in order, each bounded by $PARSER_CHECK_TIMEOUT seconds (default
# 1200) when timeout(1) is available:
#
# 1. Base guard. The base is origin/main, and the script refuses to run
#    unless the local origin/main equals the remote's main (checked with
#    `git ls-remote`, which changes nothing) and HEAD contains it. With
#    --base COMMIT the base is named explicitly and only containment is
#    checked. Both commit ids are printed. The candidate is the working tree,
#    uncommitted changes included, or with --candidate COMMIT that commit,
#    which must contain the base; that re-runs a merged fix's verification
#    with today's tools.
# 2. Builds zsh-lint-survey from the base (a `git archive` export under DIR)
#    and from the candidate. Every comparison runs through the working
#    tree's zsh-lint-survey with -candidate, so its classes and table are
#    today's whichever commits are compared.
# 3. Grids: every --rows FILE (one source per line) and --bodies FILE
#    (zsh-lint-probe body file) becomes a probe grid, judged with
#    `-compare -native -runtime`, and writes DIR/grid-N.md, the Markdown
#    table of its changed rows. -runtime runs rows with `zsh -f`, so give it
#    probe rows only.
# 4. Differentials with `-compare -native` (never run): the corpus fixtures,
#    every --root PATH (all regular files under it), every --list FILE (the
#    paths it names, one per line or NUL-separated), and with
#    --regression-corpus DIR the Corpus Gate's own regression-corpus.sh over
#    DIR/<path>/<root> as regression-corpus.txt lists them. The corpus
#    fixtures and the manifest are read from the candidate's tree.
# 5. Go checks on the working tree: go build, go vet, go test, the parser
#    fork's tests as Go CI runs them, and golangci-lint with the toolchain
#    agent-setup.sh names. --skip-go-checks leaves them to CI; with
#    --candidate they are skipped, since they would not test that commit.
#
# It prints one line per step and writes DIR/summary.md. Exit status: 0 when
# every step passed, 1 when a step failed (a REGRESSED or FALSE-ACCEPT file,
# a failing check, a timeout, a missing tool), 2 when it refused to run or a
# build failed.

set -uo pipefail
export LC_ALL=C

usage() {
  cat >&2 <<'EOF'
usage: parser-check.sh --out DIR [--base COMMIT] [--candidate COMMIT] [--rows FILE]... [--bodies FILE]...
                       [--root PATH]... [--list FILE]... [--regression-corpus DIR] [--skip-go-checks]
       parser-check.sh --check-base [--base COMMIT] [--candidate COMMIT]
EOF
  exit 2
}

say() { printf 'parser-check: %s\n' "$*"; }
die() {
  printf 'parser-check: %s\n' "$*" >&2
  exit 2
}

out='' base_ref='' candidate_ref='' check_base_only='' skip_go='' regression_corpus=''
rows=() bodies=() roots=() lists=()
while (($#)); do
  case $1 in
    --out)
      (($# > 1)) || usage
      out=$2
      shift 2
      ;;
    --candidate)
      (($# > 1)) || usage
      candidate_ref=$2
      shift 2
      ;;
    --base)
      (($# > 1)) || usage
      base_ref=$2
      shift 2
      ;;
    --rows)
      (($# > 1)) || usage
      rows+=("$2")
      shift 2
      ;;
    --bodies)
      (($# > 1)) || usage
      bodies+=("$2")
      shift 2
      ;;
    --root)
      (($# > 1)) || usage
      roots+=("$2")
      shift 2
      ;;
    --list)
      (($# > 1)) || usage
      lists+=("$2")
      shift 2
      ;;
    --regression-corpus)
      (($# > 1)) || usage
      regression_corpus=$2
      shift 2
      ;;
    --skip-go-checks)
      skip_go=1
      shift
      ;;
    --check-base)
      check_base_only=1
      shift
      ;;
    -h | --help) usage ;;
    *)
      printf 'parser-check: unknown argument: %s\n' "$1" >&2
      usage
      ;;
  esac
done
[[ -n $check_base_only || -n $out ]] || usage

root=$(git rev-parse --show-toplevel 2>/dev/null) || die "not inside a Git checkout"
cd "$root" || die "cannot enter $root"

# Resolve every input path before the script changes directory.
absolute() { [[ $1 == /* ]] && printf '%s\n' "$1" || printf '%s/%s\n' "$PWD" "$1"; }
for list in rows bodies roots lists; do
  declare -n entries=$list
  for i in "${!entries[@]}"; do
    [[ -e ${entries[i]} ]] || die "no such file or directory: ${entries[i]}"
    entries[i]=$(absolute "${entries[i]}")
  done
  unset -n entries
done
if [[ -n $regression_corpus ]]; then
  [[ -d $regression_corpus ]] || die "no such directory: $regression_corpus"
  regression_corpus=$(absolute "$regression_corpus")
fi

# Step 1: the base guard.
if [[ -n $base_ref ]]; then
  base=$(git rev-parse --verify --quiet "$base_ref^{commit}") || die "--base $base_ref is not a commit"
  base_note="named with --base $base_ref"
else
  base=$(git rev-parse --verify --quiet 'refs/remotes/origin/main^{commit}') ||
    die "no origin/main; fetch it, or name the base with --base COMMIT"
  remote=$(git ls-remote --exit-code origin refs/heads/main 2>/dev/null | cut -f1) ||
    die "cannot read origin's main to check that origin/main is current; name the base with --base COMMIT"
  if [[ $remote != "$base" ]]; then
    die "origin/main is ${base:0:12} but origin's main is ${remote:0:12}; run \`git fetch origin\` and rebase, or name the base with --base COMMIT"
  fi
  base_note="origin/main, current with origin"
fi
if [[ -n $candidate_ref ]]; then
  head=$(git rev-parse --verify --quiet "$candidate_ref^{commit}") || die "--candidate $candidate_ref is not a commit"
  head_note="named with --candidate $candidate_ref"
  what="the candidate"
else
  head=$(git rev-parse HEAD)
  head_note="HEAD"
  if [[ -n $(git status --porcelain --untracked-files=no) ]]; then
    head_note="HEAD plus uncommitted changes"
  fi
  what="HEAD"
fi
git merge-base --is-ancestor "$base" "$head" ||
  die "$what ${head:0:12} does not contain the base ${base:0:12}; rebase onto it first, or its merged fixes will read as regressions"
say "base:      $base ($base_note)"
say "candidate: $head ($head_note)"
[[ -z $check_base_only ]] || exit 0

command -v go >/dev/null 2>&1 || die "go is required"
command -v zsh >/dev/null 2>&1 || die "zsh is required for the native verdicts; run .github/scripts/agent-setup.sh"
[[ ! -e $out ]] || die "$out already exists; give a new directory so results never mix"
mkdir -p "$out/logs" "$out/work" || die "cannot create $out"
out=$(cd "$out" && pwd)
work=$out/work

step_timeout=${PARSER_CHECK_TIMEOUT:-1200}
bounded=()
if command -v timeout >/dev/null 2>&1; then
  bounded=(timeout "$step_timeout")
else
  say "timeout(1) is missing; steps are not bounded"
fi

results=()
failed=0
# record NAME STATUS DETAIL
record() {
  local result
  case $2 in
    0) result=pass ;;
    124) result=timeout ;;
    *) result="fail ($2)" ;;
  esac
  ((${2} == 0)) || failed=1
  results+=("$1"$'\t'"$result"$'\t'"$3")
  printf '%-28s %-10s %s\n' "$1" "$result" "$3"
}
# run NAME [--in DIR] COMMAND...: log to $out/logs/NAME.log and record the
# status. --in runs the command in DIR.
run() {
  local name=$1 status=0 dir=.
  shift
  if [[ $1 == --in ]]; then
    dir=$2
    shift 2
  fi
  # shellcheck disable=SC2016 # expanded by the inner bash
  "${bounded[@]}" bash -c 'cd -- "$1" || exit 2; shift; exec "$@"' run "$dir" "$@" >"$out/logs/$name.log" 2>&1 || status=$?
  record "$name" "$status" "$(tail -n 1 "$out/logs/$name.log")"
}

# Step 2: builds.
say "building the base and the candidate"
mkdir -p "$work/base" || die "cannot create $work/base"
git archive "$base" | tar -x -C "$work/base" || die "cannot export the base ${base:0:12}"
(cd "$work/base" && go build -o "$work/survey-base" ./cmd/zsh-lint-survey) >"$out/logs/build-base.log" 2>&1 ||
  die "the base does not build; see $out/logs/build-base.log"
candidate_dir=.
if [[ -n $candidate_ref ]]; then
  candidate_dir=$work/candidate
  mkdir -p "$candidate_dir" || die "cannot create $candidate_dir"
  git archive "$head" | tar -x -C "$candidate_dir" || die "cannot export the candidate ${head:0:12}"
fi
if ! (cd "$candidate_dir" && go build -o "$work/survey-candidate" ./cmd/zsh-lint-survey) >"$out/logs/build-candidate.log" 2>&1; then
  die "the candidate does not build; see $out/logs/build-candidate.log"
fi
if ! go build -o "$work/survey-tools" ./cmd/zsh-lint-survey >"$out/logs/build-tools.log" 2>&1 ||
  ! go build -o "$work/zsh-lint-probe" ./cmd/zsh-lint-probe >>"$out/logs/build-tools.log" 2>&1; then
  die "the working tree's tools do not build; see $out/logs/build-tools.log"
fi

survey=("$work/survey-tools" -compare "$work/survey-base" -candidate "$work/survey-candidate" -native)

# Step 3: grids.
grid=0
grid_step() {
  local kind=$1 file=$2 files
  grid=$((grid + 1))
  "$work/zsh-lint-probe" "-$kind" "$file" -out "$work/grid-$grid" >"$out/logs/grid-$grid-generate.log" 2>&1 || {
    record "grid-$grid" 2 "cannot generate from $file; see logs/grid-$grid-generate.log"
    return
  }
  mapfile -d '' files < <(find "$work/grid-$grid" -type f -name '*.zsh' -print0 | sort -z)
  run "grid-$grid" "${survey[@]}" -runtime -table "$out/grid-$grid.md" "${files[@]}"
}
for file in "${rows[@]}"; do grid_step rows "$file"; done
for file in "${bodies[@]}"; do grid_step bodies "$file"; done

# Step 4: differentials.
mapfile -d '' corpus < <(find "$candidate_dir/internal/survey/testdata/corpus" -type f -name '*.zsh' -print0 | sort -z)
run corpus "${survey[@]}" "${corpus[@]}"
index=0
for path in "${roots[@]}"; do
  index=$((index + 1))
  mapfile -d '' files < <(find "$path" -name .git -prune -o -type f -print0 | sort -z)
  if ((${#files[@]} == 0)); then
    record "root-$index" 2 "no files under $path"
    continue
  fi
  run "root-$index" "${survey[@]}" "${files[@]}"
done
index=0
for list in "${lists[@]}"; do
  index=$((index + 1))
  if (($(tr -dc '\000' <"$list" | wc -c) > 0)); then
    mapfile -d '' files <"$list"
  else
    mapfile -t files < <(grep -v '^$' "$list")
  fi
  if ((${#files[@]} == 0)); then
    record "list-$index" 2 "no paths in $list"
    continue
  fi
  # A missing file fails to open in both builds and would read as unchanged.
  missing=0
  for file in "${files[@]}"; do
    [[ -f $file ]] || missing=$((missing + 1))
  done
  if ((missing > 0)); then
    record "list-$index" 2 "$missing of ${#files[@]} paths in $list are not files"
    continue
  fi
  run "list-$index" "${survey[@]}" "${files[@]}"
done
if [[ -n $regression_corpus ]]; then
  while read -r path _ revision _; do
    [[ -n $path ]] || continue
    # Only a checkout rooted at the source is checked; git -C would
    # otherwise report an enclosing repository's commit.
    actual=''
    if [[ -e $regression_corpus/$path/.git ]]; then
      actual=$(git -C "$regression_corpus/$path" rev-parse HEAD 2>/dev/null) || actual=''
    fi
    if [[ $actual != "$revision" ]]; then
      say "warning: $regression_corpus/$path is ${actual:+at }${actual:-not a Git checkout}, regression-corpus.txt pins $revision"
    fi
  done <"$candidate_dir/docs/project/regression-corpus.txt"
  # regression-corpus.sh is the Corpus Gate's own step: it runs the
  # candidate's $RUNNER_TEMP/survey-head against $RUNNER_TEMP/survey-base.
  mkdir -p "$work/regression" || die "cannot create $work/regression"
  ln -s "$work/survey-candidate" "$work/survey-head"
  ln -s "$(cd "$candidate_dir" && pwd)" "$work/regression/zsh-lint"
  ln -s "$regression_corpus" "$work/regression/corpus"
  run regression-corpus --in "$work/regression" env RUNNER_TEMP="$work" \
    GITHUB_STEP_SUMMARY="$out/regression-corpus.md" bash "$root/.github/scripts/regression-corpus.sh"
fi

# Step 5: Go checks.
if [[ -n $candidate_ref ]]; then
  say "Go checks skipped: they run on the working tree, not on --candidate $candidate_ref"
elif [[ -z $skip_go ]]; then
  run go-build go build ./...
  run go-vet go vet ./...
  run go-test go test ./...
  run fork-test --in third_party/mvdan-sh go test ./syntax/ ./pattern/ ./expand/
  lint_toolchain="go$(go list -m -f '{{.GoVersion}}')"
  lint_minor=${lint_toolchain%.*}
  local_go=$(GOTOOLCHAIN=local go env GOVERSION)
  lint_env=()
  if [[ $local_go != "$lint_minor" && $local_go != "$lint_minor".* && $local_go != "$lint_minor"-* ]]; then
    lint_env=(GOTOOLCHAIN="$lint_toolchain")
  fi
  gobin=$(go env GOBIN)
  [[ -n $gobin ]] || gobin="$(go env GOPATH)/bin"
  lint=$(command -v golangci-lint 2>/dev/null) || lint=$gobin/golangci-lint
  if [[ -x $lint ]]; then
    run golangci-lint env "${lint_env[@]}" "$lint" run ./...
  else
    record golangci-lint 127 "not installed; run .github/scripts/agent-setup.sh"
  fi
fi

{
  echo "# Parser check"
  echo
  echo "Base: \`$base\` ($base_note)."
  echo "Candidate: \`$head\` ($head_note)."
  echo
  echo "| Step | Result | Detail |"
  echo "| --- | --- | --- |"
  for entry in "${results[@]}"; do
    IFS=$'\t' read -r name result detail <<<"$entry"
    printf '| %s | %s | %s |\n' "$name" "$result" "${detail//|/\\|}"
  done
  if ((grid > 0)); then
    echo
    echo "Changed grid rows: $(for ((i = 1; i <= grid; i++)); do printf '[grid-%d.md](grid-%d.md) ' "$i" "$i"; done)"
  fi
} >"$out/summary.md"
say "summary: $out/summary.md"
exit "$failed"
