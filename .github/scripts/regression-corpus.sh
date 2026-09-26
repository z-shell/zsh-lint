#!/usr/bin/env bash
# Compare parser verdicts on the regression corpus (Corpus Gate, #470).
#
# Dialect: Bash 4.0 or later (mapfile); GitHub's ubuntu-latest runner ships
# Bash 5. Run from the directory that holds zsh-lint/ and corpus/. Reads
# zsh-lint/docs/project/regression-corpus.txt, whose lines are
# `<path> <repository> <sha> <root>...`, and surveys every file under
# corpus/<path>/<root> with $RUNNER_TEMP/survey-head -compare
# $RUNNER_TEMP/survey-base -native.
#
# The regression corpus holds sources that still have open parser gaps, so a
# file that fails on both builds is a known gap and passes. The script fails
# when the change makes a file that native Zsh accepts stop parsing
# (REGRESSED) or makes a file that native Zsh rejects start parsing
# (FALSE-ACCEPT): exit 1. A missing root, an empty file set or a comparison
# that could not run exits 2.

set -uo pipefail
export LC_ALL=C

summary=${GITHUB_STEP_SUMMARY:-/dev/stdout}
tmp=${RUNNER_TEMP:?RUNNER_TEMP must be set}
manifest=zsh-lint/docs/project/regression-corpus.txt

if [[ ! -r $manifest ]]; then
  echo "::error title=Regression corpus::$manifest is not readable"
  exit 2
fi

files=()
while read -r path repository revision roots; do
  [[ -n $path ]] || continue
  if [[ -z $repository || -z $revision || -z $roots ]]; then
    echo "::error title=Regression corpus::malformed manifest line for $path"
    exit 2
  fi
  for root in $roots; do
    if [[ ! -e corpus/$path/$root ]]; then
      echo "::error title=Regression corpus::root does not exist: corpus/$path/$root"
      exit 2
    fi
    mapfile -d '' found < <(find "corpus/$path/$root" -type f -print0 | sort -z)
    files+=("${found[@]}")
  done
done <"$manifest"

if ((${#files[@]} == 0)); then
  echo "::error title=Regression corpus::no files to compare"
  exit 2
fi
echo "Comparing ${#files[@]} regression corpus file(s)."

status=0
"$tmp/survey-head" -compare "$tmp/survey-base" -native "${files[@]}" >"$tmp/regression-corpus.txt" 2>&1 || status=$?
cat "$tmp/regression-corpus.txt"

{
  echo "## Regression corpus"
  echo
  echo "${#files[@]} file(s) compared against the base build, judged by \`zsh -f -n\`."
  echo
  echo '```text'
  grep -E '^(FIXED|REGRESSED|FALSE-ACCEPT|REJECTED|MOVED) ' "$tmp/regression-corpus.txt" | head -n 50
  tail -n 1 "$tmp/regression-corpus.txt"
  echo '```'
} >>"$summary"

case $status in
  0) ;;
  1)
    echo "::error title=Regression corpus::a file regressed or a false accept was introduced; see the REGRESSED and FALSE-ACCEPT lines"
    ;;
  *)
    echo "::error title=Regression corpus::the comparison could not run (status $status)"
    ;;
esac
exit "$status"
