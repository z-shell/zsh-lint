---
description: "Guidelines for writing semantic analysis rules and AST traversals in Go using mvdan/sh"
applyTo: "internal/analyzer/**,internal/rules/**"
---

<!-- PROJECT KNOWLEDGE {"project_revision":"7ff05091a5489becf32069db1738d38ec23c8fd5","project_source_blob":"401c00159ccb871763b3aaaecfb7d7b0c891ec6b","repository":"z-shell/zsh-lint","revision":"21693189f16af6810e54f04b47789aef76dc5db3","source":"knowledge/domains/tooling/zsh-lint-go-ast.md","source_blob":"bff7df62645be1a406f4950ef9419365e884a79b","target":".github/instructions/go-ast-linting.instructions.md"} -->

# Go AST Linting & Semantic Analysis

This organization source supplies complete native project guidance through the approved records in `knowledge/project-delivery.json`. Edit the organization source, then publish and approve its revision before regenerating a project consumer; the generated consumer is not independently editable. Project instructions retain their existing authoring ownership until approved source publication, complete delivery and compatibility checks pass. Reconciled against project revision `7ff05091a5489becf32069db1738d38ec23c8fd5`; repository-relative code, command and fixture paths below refer to that project.

These instructions dictate how to build the semantic analyzer engine and lint rules for `zsh-lint` using the `mvdan/sh` parser.

## 1. The Rule Interface

Rules implement `analyzer.Rule` in [`internal/analyzer/rule.go`](../../internal/analyzer/rule.go): `ID()` returns the stable `category/rule-name` slug, `Name()` the human-readable name, and `Analyze(ctx *Context, node syntax.Node)` reports diagnostics through the context.
Optional interfaces in the same file extend a rule: `FileRule` for file-level findings, `ScopeAwareRule` to opt into the declaration index, and `ProjectRule` for invariants across configured sources.
Read that file rather than a copy here; it is the contract the engine drives.

Register new rules in [`internal/rules/rules.go`](../../internal/rules/rules.go) (`Default()` or a versioned profile) and document them per [`docs/project/rule-policy.md`](../../docs/project/rule-policy.md).
The doc comment links the released Zsh manual or Plugin Standard section it relies on; `internal/manualcite` fails without one.

## 2. AST Traversal (The Visitor Pattern)

Do not write custom recursive descent walkers unless absolutely necessary.
Rely on `syntax.Walk` from `mvdan/sh/syntax`.

```go
// Good: Using the standard walker
syntax.Walk(file, func(node syntax.Node) bool {
    switch x := node.(type) {
    case *syntax.CallExpr:
        // Handle command calls
    case *syntax.Assign:
        // Handle assignments
    }
    return true // continue traversal
})
```

## 3. Extracting Text from Nodes

Shell grammar wraps text in multiple layers (e.g., `Word` -> `Lit`).
Never cast blindly.
Use the parser's printer or explicit type checks to extract text cleanly.

```go
// Extracting literal text safely
if word, ok := node.(*syntax.Word); ok {
    if len(word.Parts) == 1 {
        if lit, ok := word.Parts[0].(*syntax.Lit); ok {
            return lit.Value
        }
    }
}
```

## 4. Testing Rules

Linter TDD requires table-driven tests that parse a Zsh string, run the rule, and assert against expected diagnostics.

```go
func TestMyRule(t *testing.T) {
    tests := []struct{
        name     string
        code     string
        expected int // number of diagnostics
    }{
        {"valid", "echo 'hello'", 0},
        {"invalid", "bad_command 'hello'", 1},
    }

    // ... setup parser, feed syntax.File to rule, assert slice length
}
```

## 5. Defensive Programming

AST nodes from `mvdan/sh` can have `nil` pointers depending on the syntax parsed (e.g., a command with no arguments, or an assignment without a value). **Always perform nil checks** before dereferencing node properties.
