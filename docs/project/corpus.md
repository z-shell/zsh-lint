# Reference Corpus

Tracking issues: [#9](https://github.com/z-shell/zsh-lint/issues/9) and
[#132](https://github.com/z-shell/zsh-lint/issues/132).

This is the explicit, reproducible set of real Z-Shell sources used to
evaluate both the parser front end (`cmd/zsh-lint-survey`) and the semantic
analyzer (`cmd/zsh-lint`). Survey runs and parser-gap issues must reference
corpus entries by repository and path so results stay comparable across runs
and front ends ([#17](https://github.com/z-shell/zsh-lint/issues/17)).

`corpus-paths.txt` is the machine-readable path inventory. This document owns
the rationale for those entries. `.github/workflows/corpus-gate.yml` checks out
the six repositories at their `main` branches, records the resolved revisions,
and applies the strict gate to the exact discovered file set.

## Layout assumption

Entries are paths relative to a checkout root containing the listed
repositories as sibling directories named after the repository. Set
`CORPUS_ROOT` to that root and clone each repository under it, then run the
gate commands described below.

## Inventory

| Repository                      | Files                                                                        | Rationale                                                                                                                                                                      |
| ------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `z-shell/src`                   | `public/zsh/init.zsh`                                                        | Zi loader; heaviest real-world Zsh (parameter-expansion flags, `always` blocks).                                                                                               |
| `z-shell/zd`                    | `docker/utils.zsh`, `docker/zshrc`, `docker/zshenv`                          | CI bootstrap Zsh; mixes POSIX-ish and Zsh-native style.                                                                                                                        |
| `z-shell/zunit`                 | `build.zsh`                                                                  | Build script; representative tooling Zsh.                                                                                                                                      |
| `z-shell/z-a-meta-plugins`      | `z-a-meta-plugins.plugin.zsh`, `functions/` (dot-prefixed handler functions) | Annex entry plus handler functions using the strict-emulation pattern.                                                                                                         |
| `z-shell/zsh-fancy-completions` | `zsh-fancy-completions.plugin.zsh`, `functions/`, `lib/`                     | Completion-style plugin; globbing, zstyle, and completion-discovery heavy.                                                                                                     |
| `z-shell/zsh-eza`               | `zsh-eza.plugin.zsh`, `functions/` (dot-prefixed handler function)           | Small, typical plugin entry file plus a strict-emulation handler function, the same pattern `z-a-meta-plugins` was included for; omitted from the initial corpus by oversight. |

Inclusion rationale, per family: the corpus deliberately spans the loader
(`src`), the CI environment (`zd`), test tooling (`zunit`), an annex
(`z-a-meta-plugins`), and user-facing plugins (completions, eza) so parser
gaps found here generalize across the organization's Zsh styles.

## Running the gate

The `Corpus Gate` workflow is the canonical automated execution. It runs on
relevant `zsh-lint` changes, weekly to detect consumer drift, and by manual
dispatch. It performs all of these checks:

- every listed root exists and expands to the reviewed file count;
- no source contains a `zsh-lint disable=` suppression;
- native `zsh -f -n` accepts every file;
- the parser survey reports no failures; and
- the semantic analyzer reports zero errors and zero warnings.

For a local run, arrange the repositories as siblings under `$CORPUS_ROOT`,
build `cmd/zsh-lint-survey` and `cmd/zsh-lint`, then execute the same commands
from the workflow. Directory entries are passed through NUL-delimited
`find -type f`, which includes dot-prefixed extensionless function files.

## Changing the corpus

Add or remove roots by updating `corpus-paths.txt` and the matching rationale
row in this file. Review any changed discovered-file count explicitly and
update the workflow's expected count in the same change. A fresh parser-only
survey is insufficient: every corpus change must re-run the complete native,
parser, analyzer, and no-suppression gate. Reports under `docs/project/` record
the revisions they ran against, so older reports stay interpretable.
