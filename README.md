# Zsh Lint

A Zi-aware linter for Zsh configuration and scripting.

> 🚧 **Reboot in progress.** `zsh-lint` is being rebooted as a **Go-based semantic
> analyzer** for Zsh (see [#5](https://github.com/z-shell/zsh-lint/issues/5)). The
> Go foundation lives under `cmd/` and `internal/`; the parser front end uses
> [`mvdan/sh`](https://github.com/mvdan/sh). The legacy Zsh plugin has been moved
> to [`legacy/`](legacy/) and is no longer part of the active product surface.

## Usage (parser survey)

`zsh-lint` currently operates as a **parser survey** tool: it parses each given
Zsh file and reports whether the front end (`mvdan/sh`, bash variant) accepts
it. It implements no lint rules yet — that work is tracked in
[#7](https://github.com/z-shell/zsh-lint/issues/7) and
[#18](https://github.com/z-shell/zsh-lint/issues/18).

```sh
go build -o zsh-lint ./cmd/zsh-lint
./zsh-lint path/to/file.zsh another.zsh
```

Each file gets an `OK`/`FAIL` line (failures include a greppable
`path:line:col: message`), followed by a summary. The exit code is `0` only if
every file parsed.

### Use in another z-shell repo (CI)

Call the reusable workflow from any repository:

```yaml
jobs:
  zsh-parser-survey:
    uses: z-shell/zsh-lint/.github/workflows/parser-survey.yml@main
    with:
      fail-on-parse-error: false
```

## 📖 Documentation

The canonical documentation for `zsh-lint`, including installation and usage guides, has moved to the **Z-Shell Wiki**:

👉 **[Z-Shell Wiki: Zsh Lint Documentation](https://wiki.zshell.dev/ecosystem/plugins/zsh_lint)**

---

## License

Copyright (c) 2021 Z-Shell Community
Licensed under the GPL-3.0 License.
