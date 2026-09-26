# Reference Corpus

Tracking issues: [#9](https://github.com/z-shell/zsh-lint/issues/9) and [#132](https://github.com/z-shell/zsh-lint/issues/132).

This is the explicit, reproducible set of real Z-Shell sources used to evaluate both the parser front end (`cmd/zsh-lint-survey`) and the semantic analyzer (`cmd/zsh-lint`).
Survey runs and parser-gap issues must reference corpus entries by repository and path so results stay comparable across runs and front ends ([#17](https://github.com/z-shell/zsh-lint/issues/17)).

`corpus-paths.txt` is the machine-readable path inventory and `corpus-revisions.txt` pins each repository to the reviewed commit.
This document owns the rationale for those entries.
On pull requests and pushes, `.github/workflows/corpus-gate.yml` checks out the six repositories at their pinned revisions, records the resolved revisions, and applies the strict gate to the exact discovered file set.
The weekly schedule and a manual dispatch check out `main` instead, so a failure there is the drift signal that a consumer changed and a reviewed revision bump is due; an ordinary zsh-lint pull request is never failed by an unrelated consumer change ([#291](https://github.com/z-shell/zsh-lint/issues/291)).

`corpus-configs/` contains reviewed, non-enrollment configurations for a second configured-profile pass.
The workflow copies each fixture into its repository checkout so configuration-relative containment remains identical to a real invocation.
These fixtures do not enroll or modify the source repositories.
Repository enrollment remains separate work under #138.

## Layout assumption

Entries are paths relative to a checkout root containing the listed repositories as sibling directories named after the repository.
Set `CORPUS_ROOT` to that root and clone each repository under it, then run the gate commands described below.

## Inventory

| Repository                      | Files                                                                        | Rationale                                                                                                                                                                      |
| ------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `z-shell/src`                   | `public/zsh/init.zsh`                                                        | Zi loader; heaviest real-world Zsh (parameter-expansion flags, `always` blocks).                                                                                               |
| `z-shell/zd`                    | `docker/utils.zsh`, `docker/zshrc`, `docker/zshenv`                          | CI bootstrap Zsh; mixes POSIX-ish and Zsh-native style.                                                                                                                        |
| `z-shell/zunit`                 | `build.zsh`                                                                  | Build script; representative tooling Zsh.                                                                                                                                      |
| `z-shell/z-a-meta-plugins`      | `z-a-meta-plugins.plugin.zsh`, `functions/` (dot-prefixed handler functions) | Annex entry plus handler functions using the strict-emulation pattern.                                                                                                         |
| `z-shell/zsh-fancy-completions` | `zsh-fancy-completions.plugin.zsh`, `lib/`                                   | Completion-style plugin; globbing, zstyle, and completion-discovery heavy.                                                                                                     |
| `z-shell/zsh-eza`               | `zsh-eza.plugin.zsh`, `functions/` (dot-prefixed handler function)           | Small, typical plugin entry file plus a strict-emulation handler function, the same pattern `z-a-meta-plugins` was included for; omitted from the initial corpus by oversight. |

Inclusion rationale, per family: the corpus deliberately spans the loader (`src`), the CI environment (`zd`), test tooling (`zunit`), an annex (`z-a-meta-plugins`), and user-facing plugins (completions, eza) so parser gaps found here generalize across the organization's Zsh styles.

### Configured survey metadata

The configured fixtures use the narrowest project kind and source profile supported by current repository evidence.
Their compatibility floors have explicit provenance:

| Repository              | Configured floor | Evidence                                                                                                           |
| ----------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------ |
| `src`                   | 5.8.1            | `public/sh/install_zpmod.sh` declares `ZSH_REQUIRED="5.8.1"`.                                                      |
| `zd`                    | 5.5.1            | The documented and automated compatibility matrix starts at 5.5.1.                                                 |
| `zunit`                 | 5.5.1            | `.github/workflows/test-matrix.yml` starts its compatibility matrix at 5.5.1.                                      |
| `z-a-meta-plugins`      | 5.9.2            | Conservative non-enrollment survey floor backed by native 5.9.2 validation; no lower repository floor is inferred. |
| `zsh-fancy-completions` | 5.9.2            | Conservative non-enrollment survey floor backed by native 5.9.2 validation; no lower repository floor is inferred. |
| `zsh-eza`               | 5.9.2            | Conservative non-enrollment survey floor backed by native 5.9.2 validation; no lower repository floor is inferred. |

The 5.9.2 survey values are not compatibility claims for older versions and must not be copied into repository enrollment without repository-owned review.

## Running the gate

The `Corpus Gate` workflow is the canonical automated execution.
It runs on relevant `zsh-lint` changes against the pinned revisions, and weekly and by manual dispatch against consumer `main` to detect drift.
It performs all of these checks:

- every listed root exists and expands to the reviewed file count;
- native `zsh -f -n` accepts every file;
- the parser survey reports no failures; and
- the semantic analyzer runs with `--no-config` and reports zero errors and zero warnings.

The independent `Configured corpus` job then runs one explicit configuration per repository and aggregates deterministic JSON. Every diagnostic must match the reviewed identity in `configured-corpus-expected.json`.
Info and Hint findings remain advisory.
Warning findings are admitted only as exact, issue-backed Standard 2 migration debt.
Configuration errors, parser errors, error diagnostics, unknown findings, disappeared findings, or line drift fail the job and require review.
The expected file records a non-empty rationale for every known finding and no source suppression is introduced merely to silence the configured corpus.
Removing an expected finding is part of completing its owning migration issue, not a compatibility promise.

Suppressions are evaluated by the analysis profile that owns the suppressed rule.
The unconfigured reference pass does not reject a directive merely because its configured-only rule is inactive; the configured pass still fails unknown or stale expected diagnostics.

The expected identities track the revisions pinned in `corpus-revisions.txt`.
When a consumer fixes or moves an admitted finding, refresh the expected file only after comparing the old and new consumer revisions and confirming that the analyzer change did not cause the difference.

For a local run, arrange the repositories as siblings under `$CORPUS_ROOT`, build `cmd/zsh-lint-survey` and `cmd/zsh-lint`, then execute the same commands from the workflow.
Directory entries are passed through NUL-delimited `find -type f`, which includes dot-prefixed extensionless function files.

## Changing the corpus

Add or remove roots by updating `corpus-paths.txt`, the matching rationale row in this file, the affected configured fixture, and the configured diagnostic classification.
Review any changed discovered-file count explicitly and update the workflow's expected count in the same change.
A fresh parser-only survey is insufficient: every corpus change must re-run the complete native, parser, unconfigured analyzer, configured analyzer, classification, and profile-owned suppression checks.
Reports under `docs/project/` record the revisions they ran against, so older reports stay interpretable.

### Bumping a consumer revision

A consumer change reaches the gate only through `corpus-revisions.txt`.
When the weekly `main` run fails, or when a consumer fix is wanted in the corpus:

1. Compare the old and new consumer revisions (`git log --stat <old>..<new>` over the roots in `corpus-paths.txt`) so that a changed file count or a moved finding is attributed to the consumer, not to the analyzer.
2. Update the pinned SHA, the workflow's `EXPECTED_CORPUS_FILES` if the count changed, and `configured-corpus-expected.json` if a classified finding moved, all in one reviewed change.
3. Re-run the complete gate locally at the new pins before opening the pull request; the pull-request run then proves the same pins in CI.

Pins are full 40-character commit SHAs, one `<repository> <sha>` line per corpus repository in the order the workflow checks them out.

## Repositories outside the strict corpus

`zi`, `zpmod`, and the untracked parts of `zunit` are deliberately not in the strict corpus yet: they still contain open parser gaps and warning-level findings, and adding them would turn a passing gate into a permanently failing one.
`.github/workflows/discovery-survey.yml` surveys them on a non-gating weekly schedule so those gaps stay visible without blocking the gate.
See [2026-08-28-discovery-survey.md](2026-08-28-discovery-survey.md).

Promotion is the same sequence used for the current members: close the parser gaps, remediate consumer findings in the owning repository, then add the roots here and re-run the complete gate.

## Regression corpus

Some sources cannot join the strict corpus because they still hold open parser gaps or warning-level findings, yet a parser change must not break the files in them that parse today.
#461 showed the cost: it made `z-shell/F-Sy-H` `functions/fsh_theme` stop parsing, and no gate saw it because F-Sy-H was in neither corpus ([#466](https://github.com/z-shell/zsh-lint/issues/466), [#470](https://github.com/z-shell/zsh-lint/issues/470)).

The `Regression corpus` job in `corpus-gate.yml` covers them on every pull request that touches the parser.
It checks out each source listed in `regression-corpus.txt` at its pinned revision, builds `zsh-lint-survey` from the pull request and from its base, and runs `.github/scripts/regression-corpus.sh`, which compares the two builds with `zsh-lint-survey -compare <base> -native`.
The job fails on a `REGRESSED` file (native Zsh accepts it, the base parsed it, the pull request does not) and on a `FALSE-ACCEPT` file (native Zsh rejects it and the pull request starts parsing it).
A file that fails on both builds is a known gap and passes; a `FIXED` or `MOVED` file is listed in the job summary for the pull request to record.

| Source                        | Pinned roots                                         | Why it is included                                                                                                                                                                                                                                                                                                                                                                                     |
| ----------------------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `z-shell/F-Sy-H`              | `F-Sy-H.plugin.zsh`, `chroma/`, `functions/`, `lib/` | Largest first-party consumer of adapter-heavy syntax: dangling operators, brace-form conditionals, and chroma pattern groups. On 2026-09-26 it holds 3 open gaps ([#481](https://github.com/z-shell/zsh-lint/issues/481), [#482](https://github.com/z-shell/zsh-lint/issues/482), [#483](https://github.com/z-shell/zsh-lint/issues/483)) and 148 warning-level findings, so it fails the strict gate. |
| `zsh-users/zsh` `Completion/` | `Completion`                                         | The upstream completion tree that `z-shell/zpmod` vendors as its `vendor/zsh` submodule, pinned to the same commit. It is not first-party code, but it is the richest native-valid grammar sample available; 110 of its 1026 native-valid files fail today.                                                                                                                                            |

Each line of `regression-corpus.txt` is `<path> <repository> <sha> <root>...`: the checkout directory under `corpus/`, the GitHub repository, a full 40-character commit SHA, and the roots below it.
The job does not track `main`: a revision bump is a reviewed change that re-runs `zsh-lint-survey -compare -native -known` at the new pin and records the result.
See [2026-09-26-regression-corpus-survey.md](2026-09-26-regression-corpus-survey.md) for the baseline.
