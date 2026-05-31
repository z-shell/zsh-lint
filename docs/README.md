# Zsh Lint

A standalone Go-based semantic analyzer for Zsh (see
[#5](https://github.com/z-shell/zsh-lint/issues/5)). It parses Zsh scripts and
reports greppable static-analysis diagnostics.

## Documentation

📖 **Canonical docs live on the Z-Shell Wiki:**
**[wiki.zshell.dev — Zsh Lint](https://wiki.zshell.dev/community/zsh_lint)**

The wiki is the single reading surface. The reference section there is generated
from this repo's Go doc comments and kept in sync automatically — do not edit it
by hand on the wiki.

## Commands

- `zsh-lint` runs the default static-analysis rules.
- `zsh-lint-survey` reports parser gaps without evaluating lint rules. The
  reusable `Parser Survey` workflow uses this command for corpus evaluation.

## For contributors

```sh
go build ./... && go vet ./... && go test ./...
go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./cmd/zsh-lint-survey ./internal/survey   # regenerate reference
```

The legacy interactive Zi/`.zshrc` plugin is archived under [`../legacy/`](../legacy/).

## License

Copyright (c) 2021 Z-Shell Community. Licensed under the GPL-3.0 License.
