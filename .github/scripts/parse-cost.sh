#!/usr/bin/env bash
# Report whole-source parse counts per file (Parse Cost workflow, #414).
#
# Dialect: Bash 4.0 or later (globstar, mapfile); GitHub's ubuntu-latest
# runner ships Bash 5. Run from the directory that holds zsh-lint/ and
# corpus/zi/. Reads $RUNNER_TEMP/survey-head and, when present,
# $RUNNER_TEMP/survey-base; writes Markdown to $GITHUB_STEP_SUMMARY and a
# ::notice for every file whose parse count rose by more than $FLAG_PERCENT
# while its verdict stayed the same. A file that starts or stops parsing
# changes its cost by definition, so it is listed, not flagged (#437).
#
# Observed, not gated (ADR-0024): the script always exits 0. A survey exit
# status of 1 only means some file does not parse yet, which is part of what
# is measured; any other status is reported and the comparison skipped.

set -u
export LC_ALL=C
shopt -s globstar nullglob

flag_percent=${FLAG_PERCENT:-10}
summary=${GITHUB_STEP_SUMMARY:-/dev/stdout}
tmp=${RUNNER_TEMP:?RUNNER_TEMP must be set}

files=(corpus/zi/zi.zsh corpus/zi/lib/**/*.zsh zsh-lint/internal/survey/testdata/corpus/*.zsh)
if ((${#files[@]} == 0)); then
  echo "::warning title=Parse cost::no files to measure"
  exit 0
fi

# trace BINARY OUTPUT: run the survey with -trace-parses and keep the
# per-file lines as "path<TAB>parses<TAB>depth<TAB>verdict", the verdict
# being OK or FAIL from the survey's report. Returns 1 when the binary could
# not produce a trace (for example a base built before #408).
trace() {
  local binary=$1 output=$2 status=0
  "$binary" -trace-parses "${files[@]}" >"$output.report" 2>"$output.raw" || status=$?
  if ((status > 1)); then
    echo "::warning title=Parse cost::$binary exited $status: $(head -n 3 "$output.raw")"
    return 1
  fi
  awk 'FNR == NR {
      if ($1 == "OK") verdict[$2] = "OK"
      else if ($1 == "FAIL") verdict[$2] = "FAIL"
      next
    }
    $1 == "TRACE" && $2 != "total" {
      sub(/^parses=/, "", $3); sub(/^adapter-depth=/, "", $4)
      print $2 "\t" $3 "\t" $4 "\t" (($2 in verdict) ? verdict[$2] : "?")
    }' "$output.report" "$output.raw" | sort >"$output"
  [[ -s $output ]]
}

if ! trace "$tmp/survey-head" "$tmp/head.tsv"; then
  echo "::warning title=Parse cost::the candidate build produced no trace"
  exit 0
fi

{
  echo "## Parse cost"
  echo
  awk -F '\t' '{ n++; p += $2; if ($3 > d) d = $3 }
    END { printf "%d files, %d whole-source parses, deepest adapter nesting %d.\n", n, p, d }' "$tmp/head.tsv"
  echo
  echo "| File | Parses | Adapter depth |"
  echo "| --- | ---: | ---: |"
  sort -t "$(printf '\t')" -k2,2nr "$tmp/head.tsv" | head -n 10 |
    awk -F '\t' '{ printf "| `%s` | %d | %d |\n", $1, $2, $3 }'
} >>"$summary"

if [[ ! -x $tmp/survey-base ]]; then
  echo >>"$summary"
  echo "No base build on this run; nothing to compare." >>"$summary"
  exit 0
fi
if ! trace "$tmp/survey-base" "$tmp/base.tsv"; then
  echo >>"$summary"
  echo "The base build produced no trace (built before #408?); comparison skipped." >>"$summary"
  exit 0
fi

# path<TAB>base parses<TAB>base depth<TAB>base verdict<TAB>head parses<TAB>
# head depth<TAB>head verdict, for files measured on both sides.
join -t "$(printf '\t')" "$tmp/base.tsv" "$tmp/head.tsv" >"$tmp/joined.tsv"

{
  echo
  echo "### Against the pull request base"
  echo
  awk -F '\t' '{ b += $2; h += $5 }
    END {
      pct = b ? (h - b) * 100 / b : 0
      printf "Total whole-source parses: %d on the base, %d here (%+.1f%%).\n", b, h, pct
    }' "$tmp/joined.tsv"
  echo
  changed=$(awk -F '\t' '$4 == $7 && ($2 != $5 || $3 != $6)' "$tmp/joined.tsv")
  if [[ -z $changed ]]; then
    echo "No file with an unchanged verdict changed its parse count or adapter depth."
  else
    echo "| File | Base parses | Parses | Base depth | Depth |"
    echo "| --- | ---: | ---: | ---: | ---: |"
    awk -F '\t' '{ printf "| `%s` | %d | %d | %d | %d |\n", $1, $2, $5, $3, $6 }' <<<"$changed" | head -n 30
  fi
  flipped=$(awk -F '\t' '$4 != $7' "$tmp/joined.tsv")
  if [[ -n $flipped ]]; then
    echo
    echo "Verdict changed (not flagged; judge these with \`zsh-lint-survey -compare -native\`):"
    echo
    echo "| File | Base | Now | Base parses | Parses |"
    echo "| --- | --- | --- | ---: | ---: |"
    awk -F '\t' '{ printf "| `%s` | %s | %s | %d | %d |\n", $1, $4, $7, $2, $5 }' <<<"$flipped" | head -n 30
  fi
} >>"$summary"

awk -F '\t' -v limit="$flag_percent" '$4 == $7 && $2 > 0 && ($5 - $2) * 100 > limit * $2 {
  printf "::notice title=Parse cost::%s: %d -> %d whole-source parses (%+.1f%%, flag above %d%%)\n", $1, $2, $5, ($5 - $2) * 100 / $2, limit
}' "$tmp/joined.tsv"

exit 0
