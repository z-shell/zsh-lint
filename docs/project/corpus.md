# Parser Evaluation Corpus

Tracking issue: [#9](https://github.com/z-shell/zsh-lint/issues/9).

This is the explicit, reproducible set of real Z-Shell sources used to
evaluate the parser front end (`cmd/zsh-lint-survey`). Survey runs and
parser-gap issues must reference corpus entries by repository and path so
results stay comparable across runs and front ends
([#17](https://github.com/z-shell/zsh-lint/issues/17)).

## Layout assumption

Entries are paths relative to a checkout root containing the listed
repositories as sibling directories named after the repository. Set
`CORPUS_ROOT` to that root and clone each repository under it, then run the
survey command shown below.

## Inventory

| Repository | Files | Rationale |
| --- | --- | --- |
| `z-shell/src` | `public/zsh/init.zsh` | Zi loader; heaviest real-world Zsh (parameter-expansion flags, `always` blocks). |
| `z-shell/zd` | `docker/utils.zsh`, `docker/zshrc`, `docker/zshenv` | CI bootstrap Zsh; mixes POSIX-ish and Zsh-native style. |
| `z-shell/zunit` | `build.zsh` | Build script; representative tooling Zsh. |
| `z-shell/z-a-meta-plugins` | `z-a-meta-plugins.plugin.zsh`, `functions/` (dot-prefixed handler functions) | Annex entry plus handler functions using the strict-emulation pattern. |
| `z-shell/zsh-fancy-completions` | `zsh-fancy-completions.plugin.zsh`, `functions/`, `lib/` | Completion-style plugin; globbing, zstyle, and completion-discovery heavy. |
| `z-shell/zsh-eza` | `zsh-eza.plugin.zsh` | Small, typical plugin entry file. |

Inclusion rationale, per family: the corpus deliberately spans the loader
(`src`), the CI environment (`zd`), test tooling (`zunit`), an annex
(`z-a-meta-plugins`), and user-facing plugins (completions, eza) so parser
gaps found here generalize across the organization's Zsh styles.

## Running the survey

From a `zsh-lint` checkout:

    cd "$CORPUS_ROOT"
    find src/public/zsh/init.zsh \
         zd/docker/utils.zsh zd/docker/zshrc zd/docker/zshenv \
         zunit/build.zsh \
         z-a-meta-plugins/z-a-meta-plugins.plugin.zsh \
         z-a-meta-plugins/functions \
         zsh-fancy-completions/zsh-fancy-completions.plugin.zsh \
         zsh-fancy-completions/functions \
         zsh-fancy-completions/lib \
         zsh-eza/zsh-eza.plugin.zsh \
         -type f | sort | xargs go run <path-to-zsh-lint>/cmd/zsh-lint-survey

Directory entries are passed through `find -type f`, which also picks up
dot-prefixed function files (the `z-a-meta-plugins` handlers).

## Changing the corpus

Add or remove entries via a pull request to this file, including a rationale
row. Survey reports under `docs/project/` record the corpus state they ran
against, so older reports stay interpretable after the corpus changes.
