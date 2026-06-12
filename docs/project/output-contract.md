# Machine-Readable Output Contract

Tracking issue: [#20](https://github.com/z-shell/zsh-lint/issues/20).

The stable transport format for diagnostics, for CI and editor consumers.
Version 1 is implemented by `internal/diag.WriteJSON` and exposed by
`zsh-lint --format=json`; the contract is covered by tests in
`internal/diag/json_test.go`.

## Versioning

The envelope carries an integer `version` (currently `1`,
`diag.JSONVersion`). Adding new optional members is allowed within a
version; removing or changing the meaning of an existing member requires
incrementing it. Consumers must reject versions they do not understand.

## Envelope

`zsh-lint --format=json <files...>` writes one JSON object to stdout,
followed by a single newline:

```json
{
  "version": 1,
  "diagnostics": [
    {
      "rule": "quoting/unquoted-var",
      "severity": "warning",
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
    "warnings": 1,
    "infos": 0,
    "hints": 0
  }
}
```

- `diagnostics` is always an array (empty, never `null`), sorted by the
  deterministic order defined by `diag.Diagnostics.Sort` (file, range,
  rule, severity, message) so output is reproducible across runs.
- `severity` is one of `error`, `warning`, `info`, `hint` — the canonical
  lowercase names from `internal/diag`, ordered per the LSP scale.
- `rule` is the stable `category/rule-name` slug (rule policy,
  `docs/project/rule-policy.md`).
- `line` and `column` are 1-based; `offset` is a 0-based byte offset.
  `range` is half-open: `[start, end)`.
- An unpositioned diagnostic (whole-file or unknown location) omits the
  `range` member entirely.
- `summary.files` counts files inspected, independent of how many produced
  findings.
- Human-readable output is unchanged and remains the default; the flag is
  opt-in.

## Parser failures

Parser failures and analyzer findings share one representation: a parser
failure is a diagnostic with the reserved rule ID **`parse/error`**,
severity `error`, a position-free `message`, and a zero-width `range` at
the failure position when the front end provides one. The `parse/` category
is reserved for the front end; analyzer rules must not use it.

## Suppression (#19)

Per the suppression contract (`docs/project/suppression.md`): suppressed
findings are omitted from `diagnostics`, while `meta/*` diagnostics
(`meta/malformed-suppression`, `meta/unused-suppression`) are always
included, so integrations can audit suppression usage. Until suppression is
implemented in the analyzer, no `meta/*` diagnostics are emitted; their
inclusion is not a version change.

## Exit codes

Unchanged by the format flag: `0` no findings at `warning` or above, `1`
findings at `warning`/`error` (including `parse/error`) or unreadable
inputs, `2` usage or encoding errors.
