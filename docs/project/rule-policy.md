# First-Wave Rule Policy

Tracking issue: [#7](https://github.com/z-shell/zsh-lint/issues/7).

How the first wave of 15–25 semantic rules is selected, what evidence a rule
must provide before it ships, and what every shipped rule must document.
Five rules predate this policy (`quoting/unquoted-var`, `security/eval`,
`style/backquotes`, `style/function-decl`, `style/prefer-double-brackets`);
they are grandfathered but must gain the documentation schema below before
the first tagged release.

## Selection criteria

A rule candidate qualifies for the first wave only if all of these hold:

1. **Corpus evidence.** The pattern the rule diagnoses occurs in the survey
   corpus (`docs/project/corpus.md`) or in a linked real-world Z-Shell
   repository. Cite files and lines in the proposal. Rules invented from
   intuition wait for a later wave.
2. **Manual grounding.** The behavior the rule warns about is explainable
   from the Zsh manual (`zshexpn`, `zshparam`, `zshmisc`, `zshoptions`) or
   the Z-Shell Plugin Standard. The proposal names the section.
3. **Parseable today.** The rule must be implementable on AST shapes the
   current front end produces. Patterns masked by open parser gaps
   (#11, #12, #13, #15, #16, #53) are deferred until the gap is resolved —
   a rule that can never see its target construct is untestable.
4. **Actionable message.** The diagnostic must tell the user what to do,
   not only what is wrong. If no safe rewrite exists, the rule must say why
   the pattern is risky.
5. **Bounded false positives.** The proposal must enumerate the known
   legitimate uses of the pattern and state, for each, whether the rule
   stays silent, downgrades severity, or relies on suppression (#19).

## Rule IDs and categories

Rule IDs are stable `category/rule-name` slugs (lowercase, hyphenated).
First-wave categories:

| Category   | Scope                                                                   |
| ---------- | ----------------------------------------------------------------------- |
| `quoting`  | Word splitting, globbing, and expansion quoting hazards.                |
| `security` | Patterns that can execute or expand untrusted input.                    |
| `style`    | Modern-Zsh idiom preferences with no behavior change.                   |
| `compat`   | Constructs that behave differently across Zsh versions/emulation modes. |
| `plugin`   | Z-Shell Plugin Standard conformance (ZERO idiom, unload, fpath).        |

New categories require updating this table in the same PR that ships the
first rule using them. A rule ID, once released, is never reused or renamed
— deprecate and introduce a new ID instead.

## Severity mapping

Severities follow the LSP scale implemented in `internal/diag`:

| Severity  | Use for                                                          |
| --------- | ---------------------------------------------------------------- |
| `error`   | Code that is broken or will misbehave at runtime in plain Zsh.   |
| `warning` | Likely-bug patterns with realistic false-positive potential.     |
| `info`    | Risky-but-sometimes-intentional patterns (e.g. `security/eval`). |
| `hint`    | Style and idiom preferences.                                     |

A first-wave rule may not ship at `error` unless its false-positive section
demonstrates zero known legitimate uses in the corpus.

## Documentation schema

Every shipped rule must provide, in its proposal issue and in the generated
reference docs (Go doc comment on the rule type):

- **ID** — the `category/rule-name` slug returned by `ID()`.
- **Name** — the human-readable name returned by `Name()`.
- **Summary** — one sentence: what the rule reports.
- **Why** — the failure mode, with the Zsh-manual or Plugin Standard
  reference.
- **Bad / Good** — at least one minimal flagged snippet and its fix.
- **Severity** — with a one-line justification per the mapping above.
- **False positives** — known legitimate uses and how the rule treats them.
- **Suppression** — how to silence an intentional finding. Until #19 lands,
  write `pending #19`; rules may not document ad-hoc escape hatches.
- **Corpus evidence** — file/line citations from the survey corpus.

## Shipping checklist

A rule PR is mergeable when:

1. The proposal issue (rule-proposal template) is approved and linked.
2. The implementation conforms to
   `.github/instructions/go-ast-linting.instructions.md` (visitor pattern,
   safe text extraction, table-driven tests).
3. Table-driven tests cover the Bad and Good examples plus every documented
   false-positive case.
4. The rule is registered in `internal/rules.Default()` and the generated
   reference docs are regenerated.
5. Running the analyzer over the survey corpus produces no finding the
   author cannot classify as true positive or documented false positive.

## False-positive policy

False positives are budgeted, not forbidden: a first-wave rule earns its
severity by how well it characterizes them. When a user reports a false
positive, the triage order is (1) narrow the rule, (2) downgrade severity,
(3) document it as a suppression case — removing the rule is a last resort
and requires a tracking issue explaining why narrowing failed.
