# Zsh Lint

A Go-based semantic analyzer for Zsh (reboot in progress, see
[#5](https://github.com/z-shell/zsh-lint/issues/5)). It currently operates as a
parser-survey tool; no lint rules yet.

## Documentation

📖 **Canonical docs live on the Z-Shell Wiki:**
**[wiki.zshell.dev — Zsh Lint](https://wiki.zshell.dev/ecosystem/plugins/zsh_lint)**

The wiki is the single reading surface. The reference section there is generated
from this repo's Go doc comments and kept in sync automatically — do not edit it
by hand on the wiki.

## For contributors

```sh
go build ./... && go vet ./... && go test ./...
go tool gomarkdoc --output ref.md ./cmd/zsh-lint ./internal/survey   # regenerate reference
```

The legacy interactive Zi/`.zshrc` plugin is archived under [`../legacy/`](../legacy/).

## License

Copyright (c) 2021 Z-Shell Community. Licensed under the GPL-3.0 License.
