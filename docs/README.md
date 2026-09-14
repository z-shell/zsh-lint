# Zsh Lint

A standalone Go-based semantic analyzer for Zsh (see
[#5](https://github.com/z-shell/zsh-lint/issues/5)). It parses Zsh scripts and
reports greppable static-analysis diagnostics.

## Documentation

The canonical user guides live on the
[Z-Shell Wiki](https://wiki.zshell.dev/community/zsh_lint). Start there for
installation, file selection, output, suppressions, CI, and troubleshooting.

The wiki's rule reference is generated from the latest published release, not
from unreleased `main` behavior. Do not edit its marked generated region by
hand.

## Copyable examples

- [`examples/standalone`](../examples/standalone) contains one executable Zsh
  script and its project configuration.
- [`examples/plugin`](../examples/plugin) classifies a plugin entrypoint,
  autoloaded function, completion, and test fixture.

The example configurations use the project-configuration support currently on
`main`, including automatic `zsh-lint.json` discovery. This behavior is newer
than the published v1.2.0 CLI; the wiki keeps the published path separate from
the unreleased project-configuration guide.

## Commands

- `zsh-lint` runs the default static-analysis rules.
- `zsh-lint-survey` reports parser gaps without evaluating lint rules. The
  reusable `Parser Survey` workflow uses this command for corpus evaluation.

## For contributors

The source-adjacent [project configuration contract](project/project-configuration.md)
documents discovery, explicit overrides, and the metadata available to
project-aware rules.

Create short-lived `feature-<id>`, `bug-<id>`, or `hotfix-<id>` branches from
`main` and target pull requests back to `main`. Reviewed changes are
squash-merged; annotated `vX.Y.Z` tags remain the publication boundary.

```sh
go build ./... && go vet ./... && go test ./...
go tool gomarkdoc --output ref.md \
  ./cmd/zsh-lint ./cmd/zsh-lint-survey ./internal/survey ./internal/rules
```

The legacy interactive Zi/`.zshrc` plugin is archived under [`../legacy/`](../legacy/).

## License

Copyright (c) 2021 Z-Shell Community. Licensed under the GPL-3.0 License.
