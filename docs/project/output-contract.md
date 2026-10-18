# Machine-Readable Output Contract

Tracking issue: [#20](https://github.com/z-shell/zsh-lint/issues/20).

The stable transport format for diagnostics, for CI and editor consumers.
Version 1 is implemented by `internal/diag.WriteJSON` and exposed by `zsh-lint --format=json`; the contract is covered by tests in `internal/diag/json_test.go`.

## Versioning

The envelope carries an integer `version` (currently `1`, `diag.JSONVersion`).
Adding new optional members is allowed within a version; removing or changing the meaning of an existing member requires incrementing it.
Consumers must reject versions they do not understand.

## Envelope

`zsh-lint --format=json <files...>` writes one JSON object to stdout, followed by a single newline:

```json
{
  "version": 1,
  "diagnostics": [
    {
      "rule": "quoting/unquoted-var",
      "severity": "info",
      "message": "Variable expansion should be double-quoted",
      "file": "lib/a.zsh",
      "range": {
        "start": { "line": 2, "column": 6, "offset": 13 },
        "end": { "line": 2, "column": 8, "offset": 15 }
      }
    }
  ],
  "summary": {
    "files": 1,
    "diagnostics": 1,
    "errors": 0,
    "warnings": 0,
    "infos": 1,
    "hints": 0
  }
}
```

- `diagnostics` is always an array (empty, never `null`), sorted by the deterministic order defined by `diag.Diagnostics.Sort` (file, range, rule, severity, message) so output is reproducible across runs.
- `severity` is one of `error`, `warning`, `info`, `hint` — the canonical lowercase names from `internal/diag`, ordered per the LSP scale.
- `rule` is the stable `category/rule-name` slug (rule policy, `docs/project/rule-policy.md`).
- `line` and `column` are 1-based; `offset` is a 0-based byte offset.
  `range` is half-open: `[start, end)`.
- An unpositioned diagnostic (whole-file or unknown location) omits the `range` member entirely.
- `summary.files` counts files inspected, independent of how many produced findings.
- Human-readable output is unchanged and remains the default; the flag is opt-in.

## Parser failures

Parser failures and analyzer findings share one representation: a parser failure is a diagnostic with the reserved rule ID **`parse/error`**, severity `error`, a position-free `message`, and a zero-width `range` at the failure position when the front end provides one.
The `parse/` category is reserved for the front end; analyzer rules must not use it.

## Suppression (#19)

Per the suppression contract (`docs/project/suppression.md`): suppressed findings are omitted from `diagnostics`, while `meta/*` diagnostics (`meta/malformed-suppression`, `meta/unused-suppression`) are always included, so integrations can audit suppression usage.
Suppression is implemented in the analyzer (`internal/suppress`, #65); the introduction of `meta/*` diagnostics was not a version change.

## Exit codes

Unchanged by the format flag: `0` no findings at `warning` or above, `1` findings at `warning`/`error` (including `parse/error`) or unreadable inputs, `2` usage or encoding errors.

## MCP stdio server

`zsh-lint mcp [--zsh PATH]` exposes exactly `zsh_lint` and `zsh_syntax` over newline-delimited JSON-RPC 2.0 on stdin/stdout. Stdout contains protocol messages only; server logging goes to stderr. There is no HTTP listener, configuration write or automatic MCP-client registration. EOF closes the server normally. Startup usage errors exit 2; a transport failure exits 1.

The official Go SDK v1.8.0 provides lifecycle and tool dispatch, including `initialize`, `notifications/initialized`, `tools/list` and `tools/call`. It supports protocol revisions from `2024-11-05` through `2026-07-28`, including `2025-11-25` initialization. Recoverable malformed frames return a JSON-RPC parse or invalid-request error and leave the session usable. Each inbound line is bounded to 16 MiB; an oversized frame terminates the connection. Tool input schemas reject unknown fields.

Request IDs must be strings or integer literals between -9007199254740991 and 9007199254740991. Larger integers, fractional or exponent-form IDs and null IDs are rejected rather than silently rounded through the SDK's numeric decoder. A rejected numeric ID is echoed unchanged in its error response; an absent or unidentifiable ID is null. Cancellation `requestId` follows the same supported-ID restriction. Use string IDs for identifiers outside that integer range.

### `zsh_lint`

Arguments are an object with required `files` (a nonempty array of nonempty file paths), optional `config` (a nonempty configuration-file path) and optional `no_config` (boolean, default false). `config` and true `no_config` are mutually exclusive. File and configuration paths resolve against the server's working directory, which does not change between calls. There is no inline source, recursive input discovery or source rewrite.

The default searches for `zsh-lint.json` upward from each file; the overrides match CLI `--config` and `--no-config`. Files in one call are analyzed together, preserving configured project-rule scope. Successful `structuredContent` is the unchanged version 1 envelope above; its first text content block is the exact CLI JSON, including its trailing newline. Diagnostic ordering, suppression and parser failures are unchanged. Findings, including `parse/error`, are successful tool results, not protocol or execution errors.

Unreadable or non-regular inputs, configuration errors and other execution failures return `isError: true` with actionable text. When the CLI produces partial diagnostics on a configuration-discovery failure, the result retains that envelope alongside the error. Unknown methods and tools or malformed method parameters return JSON-RPC errors. Neither kind of error crashes a healthy session.

### `zsh_syntax`

Arguments are an object with required `files`, using the same explicit file-path rules. It does not use lint configuration. The server finds `zsh` on PATH by default; startup `--zsh PATH` selects another executable, and tool calls cannot change it. A missing executable is a tool execution error and does not disable static linting.

`structuredContent` has a `files` array in input order. Each member contains `file`, `valid`, `syntax` and `compile`. Both checks contain `stderr` and `exit_code`; the text content block serializes the same object:

```json
{
  "files": [
    {
      "file": "negation.zsh",
      "valid": true,
      "syntax": { "stderr": "", "exit_code": 1 },
      "compile": { "stderr": "", "exit_code": 0 }
    }
  ]
}
```

`syntax` runs `zsh -f -n -- FILE`. `compile` runs the native `zcompile -U -R` builtin with an explicit output path inside a private temporary directory beneath the process's temporary-storage location (`TMPDIR` where supported). Output is removed when the call finishes, including errors. No compiled file is written beside a source. Paths are positional arguments, never interpolated into the shell command.

A file is valid exactly when both checks have empty stderr: `! true` is valid despite the syntax check's exit 1. Native syntax findings are successful tool results with `valid: false`, so consumers must inspect that field. Missing inputs, process-start failures, cancellation, signals and the 30-second native-call deadline are tool execution errors, not syntax verdicts. A normal nonzero native status alone is not an infrastructure failure.

Static linting does not execute source. Native checks do not execute the input program, but `-f` does not prevent the system `zshenv` from running, and native parsing can perform expansion. Neither native checking nor MCP tool annotations establish a sandbox for untrusted source, executable search paths or startup configuration. Both tools report read-only and idempotent hints because their intended effect is analysis; temporary compilation output is an implementation detail, not authorization to mutate source files.
