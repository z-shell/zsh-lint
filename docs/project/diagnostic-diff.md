# Diagnostic-diff verification

Use the existing verifier entry point for analyzer comparisons as well as parser verdicts. The diagnostics mode requires Git, Go and the selected source populations. It builds both real CLIs; it does not execute corpus scripts, run functional tests or run mutation. Run the repository's normal checks separately.

```bash
bash .github/scripts/verify-parser-change.sh --diagnostics \
  --population inventory=docs/project/diagnostic-populations/inventory.json \
  --population organization=docs/project/diagnostic-populations/organization.json \
  --population historical-mixed=docs/project/diagnostic-populations/configured.json \
  --population configured-only=docs/project/diagnostic-populations/configured-only.json \
  --root org=/path/to/organization/repos \
  --root ohmyzsh=/path/to/ohmyzsh \
  --root prezto=/path/to/prezto \
  --root powerlevel10k=/path/to/powerlevel10k \
  --root zsh-autosuggestions=/path/to/zsh-autosuggestions \
  --root zsh-syntax-highlighting=/path/to/zsh-syntax-highlighting \
  --root f-sy-h-upstream=/path/to/f-sy-h-upstream \
  --target-rule plugin/function-scoped-options \
  --output /path/to/evidence/new-comparison BASE
```

`--diagnostics` must be the first argument. With no explicit `BASE`, the existing verifier's origin/main freshness and ancestry guard applies. An explicit base skips only the remote freshness lookup, not ancestry. `--candidate REV` builds a historical candidate; otherwise the candidate is a snapshot of tracked and non-ignored untracked working-tree files, including local edits and deletions. Neither source revision is checked out. Source and input changes during the run invalidate its evidence.

Each `--population NAME=MANIFEST` is run once per build, preserving the sibling-file scope of configured project rules. Do not split a configured population into per-file diagnostic commands. The whole-population command can fail on the operating system's argument-size limit for larger custom populations; it reports a failed comparison, not an unchanged population.

Manifest modes select evidence requirements, not a different product configuration schema:

- `no-config` passes `--no-config` and records `configuration-disabled` for every input, without loading ancestor configurations.
- `configured` retains the historical automatic-discovery invocation, including unconfigured fallback. Its name is legacy: the report labels it historical mixed-mode compatibility evidence, not a configured-only population.
- `configured-only` uses the same whole-population automatic-discovery invocation but requires every input to resolve under its nearest configuration. Missing configurations and unmatched source roots fail preflight rather than silently contributing unconfigured evidence.

Discovery, loading and classification reuse `internal/projectconfig`, the same resolver used by the CLI. Every input records either its actual resolved profile, role, source root and project metadata, or `no-configuration`, `unmatched-source-root` or `configuration-disabled` as its fallback reason. A discovered configuration is not necessarily effective: `effective_configuration` contains only mapped inputs, while `configurations` hashes every discovered configuration, including those that leave an input unmatched. Invalid configurations fail evidence, rather than becoming fallback. The nearest configuration remains authoritative; an unmatched or invalid inner configuration does not cause a retry against an outer one.

A finding's identity includes every JSON field, including file, rule, severity, message and range. Multiset comparison preserves duplicate findings. A moved finding appears as a removal and addition, even if its rule's net count stays unchanged. Paths in normalized results use the manifest's root aliases rather than temporary build paths.

`--target-rule RULE` requires all non-target diagnostic identities to remain identical. Without it, differences are reported without an acceptance judgment about their intent. Even for a target rule, a passing comparison does not establish correctness or precision; inspect each changed identity. `parse/error` remains a diagnostic and is also counted separately for each population.

The new output directory retains `diagnostics.json`, `summary.md`, normalized lane outputs, complete command stdout/stderr, command arguments and statuses, source archives/snapshots and built CLIs. The JSON report includes per-rule counts and added/removed identities, source and manifest hashes, original input and discovered-configuration hashes, effective configuration membership and per-input resolution evidence. Resolution evidence and hashes must remain identical before and after both lanes. This is classification by the verifier checkout's resolver, not proof of historical resolver behavior for arbitrary revisions; investigate comparisons that change configuration semantics separately. These artifacts can contain local absolute paths and source copies; keep them private unless separately sanitized. Existing output directories are refused. Nothing removes historical evidence.

Exit 0 means valid comparison evidence and, when selected, an identical non-target slice. Exit 1 means a non-target mismatch. Exit 2 means invalid or unavailable evidence, including a missing input, malformed CLI output, inconsistent summary/exit status, unexpected CLI stderr, build failure or timeout. `--timeout SECONDS` bounds each managed build, base-guard and analyzer command, default 600; an execution timeout kills the command's process group and retains its command result. Output draining after process exit is bounded by the shorter of one second and the command timeout; an expired drain also invalidates the evidence. This is not a global wall-clock bound: Git bookkeeping and compilation of the Go runner occur outside the managed command runner.

## Population membership and regeneration

The original committed lists preserve the populations used for the PR #713 and PR #714 comparisons byte-for-byte: inventory 2,462 files, organization 242 and the historically configured-labelled list 69. They are measurement populations, not the separate gate-clean reference corpus. Membership is explicit and intentionally includes sources with findings and parse errors. The organization subset includes tests and fixtures; it is not a hand-picked production-only subset.

At `4ea68e2`, 23 of the historical list's 69 inputs resolve under existing configurations and 46 use unconfigured fallback. `configured-only.json` contains exactly those 23 mapped inputs, in their historical order, with mode `configured-only`. It adds truthful configured evidence without modifying the historical list or broadening consumer configurations. Its membership depends on actual source roots, not whether an ancestor `zsh-lint.json` exists. Recheck resolution and retain input/configuration hashes on every use.

The populations overlap. Repeated observations of the same finding are not independent precision samples, and the configured-only subset is not an organization-wide census. After #729 the historical populations have zero Warnings: precision is undefined, not 100%, and the 80% Warning goal remains unestablished. The verifier summary explicitly distinguishes a valid diagnostic comparison from a precision claim. Follow [the precision gate](rule-policy.md#precision-gate) for judgments and any bounded smaller-population exception; synthetic regressions cannot enlarge the organization precision denominator.

`org` maps to a directory with `annexes/`, `core/`, `env/`, `modules/`, `plugins/` and `tools/` children. External roots map directly to their project checkouts. Supply the F-Sy-H upstream under its neutral root alias. The manifests do not fetch, move or modify those checkouts. A manifest fixes membership, not source bytes: retain each comparison's hashes and exact source revisions when reusing evidence.

To regenerate a manifest from an explicitly selected newline- or NUL-separated file list, use the same entry point. Absolute list entries are mapped to roots; relative entries use `--relative-to`. Map every root represented in the list, using the same aliases as the comparison. The converter rejects missing, duplicate and unmapped inputs and refuses to overwrite an existing output.

```bash
bash .github/scripts/verify-parser-change.sh --diagnostics --write-manifest \
  --list organization-files.txt --root org=/path/to/organization/repos \
  --relative-to org --mode no-config --output organization-new.json
```

Review membership changes before replacing a committed manifest. Use `--mode configured-only` for new populations claiming configured evidence; `--mode configured` remains available to reproduce historical mixed-mode lists. The converter preserves the selected list and mode; comparison preflight enforces configured-only resolution. For inventory, also pass each external `--root` from the comparison example. Do not substitute an unrestricted recursive scan or silently drop missing entries to obtain matching counts. Preserve extensionless completion functions and explicit test fixtures. The converter preserves file-list order, so importing the original lists reproduces the committed JSON exactly.

## User-level confirmation shells

```bash
bash .github/scripts/agent-setup.sh --shells-only
```

The setup reuses compatible commands already on PATH; otherwise it downloads checksum-pinned dash 0.5.12, mksh R59c and GNU Bash 5.3 sources and builds them without privilege escalation. A C compiler, make, curl, tar and sha256sum are required for missing shells. Download/build failure is a setup failure, never permission to skip required confirmation. The dash behavioral probe, exact mksh R59 marker and Bash 5.3 marker match the fork's native-confirmation checks.

The default cache is `${XDG_CACHE_HOME:-$HOME/.cache}/zsh-lint/fork-shells`; `ZSH_LINT_SHELL_PREFIX` selects another user-owned prefix. The setup prints the PATH activation command for subsequent shells and uses that PATH internally for the full setup command. `--shells-only` performs no module download or other setup step. Build trees and downloaded archives remain in that prefix for diagnosis; no package-manager configuration or system shell is changed.

Validate the activated tools with `REQUIRE_SHELLS=1 go test ./third_party/mvdan-sh/syntax/ ./third_party/mvdan-sh/pattern/ ./third_party/mvdan-sh/expand/`. This is native confirmation on the current host, not proof of a different CI image's toolchain.
